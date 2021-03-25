package sdstore

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/karrick/godirwalk"
	"github.com/otiai10/copy"
	"github.com/screwdriver-cd/store-cli/logger"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const CompressFormatTarZst = ".tar.zst"
const CompressFormatZip = ".zip"
const CompressionLevel = -3 // default compression level - 3 / possible values (1-19) or --fast
const Md5Extension = ".md5"
const DefaultFilePermission = os.ModePerm
const ZstdCli = false // use zstd binary or go library
const FlockWaitMinSecs = 5
const FlockWaitMaxSecs = 15

type FileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modtime"`
	Mode    string `json:"mode"`
}

// ExecCommand : os exec command
var ExecCommand = exec.Command

// executeCommand : Execute shell commands
// return output => executing shell command succeeds
// return error => for any error
func executeCommand(command string) error {
	logger.Info(command)
	cmd := ExecCommand("sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil || strings.TrimSpace(stderr.String()) != "" {
		return logger.Error(fmt.Errorf("error: %v, %v, out: %v", err, stderr.String(), stdout.String()))
	}
	logger.Info("command output: " + stdout.String())
	return nil
}

// releaseLock : release lock
// return error => for any error
func releaseLock(path string) {
	_ = os.Remove(path + ".lock")
}

// acquireLock : acquire lock before overwriting file
// path => path
// read => read / write
// return error => for any error
func acquireLock(path string, read bool) error {
	rand.Seed(time.Now().UnixNano())
	attempts := 1
	for attempts <= 10 {
		if read {
			_, err := os.Lstat(path + ".lock")
			if err != nil {
				return nil
			} else {
				fmt.Printf("waiting, cache is not available yet, attempts: %v \n", attempts)
			}
		} else {
			_, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, DefaultFilePermission)
			if err == nil {
				fmt.Println("acquired lock on ", path)
				return nil
			}
			fmt.Printf("waiting to acquire lock on %v, attempts: %v \n", path, attempts)
		}
		r := FlockWaitMinSecs + rand.Intn(FlockWaitMaxSecs-FlockWaitMinSecs)
		time.Sleep(time.Duration(r) * time.Second)
		attempts++
	}
	return errors.New("max attempts exceeded")
}

// ZStandard from https://github.com/facebook/zstd
// To test in mac - download from https://bintray.com/screwdrivercd/screwdrivercd/download_file?file_path=zstd-cli-1.4.8-macosx.tar.gz and set path
// To test in linux - download from https://bintray.com/screwdrivercd/screwdrivercd/download_file?file_path=zstd-cli-1.4.8-linux.tar.gz and set path
func getZstdBinary() string {
	switch runtime.GOOS {
	case "darwin":
		return "zstd-cli-macosx"
	default:
		return "zstd-cli-linux"
	}
}

/*
Get byte array of file metadata and return md5
param - b         		byte array of file metadata
return - string 		md5 string
*/
func getMd5(b []byte) string {
	var err error

	md5hash := md5.New()
	_, err = io.Copy(md5hash, bytes.NewReader(b))
	if err != nil {
		return ""
	}
	md5hashInBytes := md5hash.Sum(nil)
	md5str := hex.EncodeToString(md5hashInBytes)
	return md5str
}

func writeMd5(dst, md5 string) {
	var (
		md5File *os.File
		err     error
		b       int
	)

	md5File, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, DefaultFilePermission)
	if err != nil {
		logger.Warn(fmt.Sprintf("unable to create %v", dst))
	} else {
		defer md5File.Close()
		if b, err = md5File.WriteString(md5); err != nil {
			logger.Warn(fmt.Sprintf("failed to write %v", dst))
		} else {
			_ = md5File.Sync()
			logger.Info(fmt.Sprintf("wrote %d bytes of %v", b, dst))
		}
	}
}

/*
Get metadata and return md5 and total size for the given path.
using file meta instead of calculating md5 for each file content, as its slower than storing the cache
param - path         		file / folder path
return - string / int64 	md5 for path / total size in bytes
*/
func getMetadataInfo(path string) ([]*FileInfo, string, int64) {
	var fileInfos []*FileInfo

	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(filePath string, de *godirwalk.Dirent) error {
			size := int64(0)
			file, err := os.Lstat(filePath)
			if err != nil || file == nil {
				return nil
			}
			if !de.IsDir() {
				size = file.Size()
			}
			fileInfos = append(fileInfos, &FileInfo{filePath, size, file.ModTime().UnixNano(), file.Mode().String()})

			return nil
		},
		ErrorCallback: func(filePath string, err error) godirwalk.ErrorAction {
			logger.Warn(err)
			return godirwalk.SkipNode
		},
		Unsorted:            false,
		AllowNonDirectory:   true,
		FollowSymbolicLinks: false,
	})

	size := int64(0)
	if err != nil {
		return fileInfos, "", size
	}

	for _, s := range fileInfos {
		size += s.Size
	}

	md5Json, _ := json.Marshal(fileInfos)
	return fileInfos, getMd5(md5Json), size
}

/*
compare md5 of files for source and destination directories
param - newMd5         	md5 of source
param - dest				all directory but the last element of path
param - destBase			last element of destination directory
return - bool   return - true (md5 same) / false (md5 changed)
*/
func compareMd5(newMd5, dest, destBase string) bool {
	var msg, oldMd5FilePath string

	oldMd5FilePath = filepath.Join(dest, fmt.Sprintf("%s%s", destBase, Md5Extension))
	oldMd5InBytes, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5InBytes = []byte("")
		msg = fmt.Sprintf("%v, not able to get %s%s from: %s", err, destBase, Md5Extension, dest)
		logger.Warn(msg)
	}
	oldMd5 := string(oldMd5InBytes)

	if strings.Compare(oldMd5, newMd5) == 0 {
		return true
	} else {
		return false
	}
}

/*
remove cache directory from shared file server
param - path			cache path
param - md5Path			md5 path
param -	command			set or remove
return - nothing
*/
func removeCacheDirectory(path, md5Path string) {
	_, err := os.Lstat(path)

	if err != nil {
		logger.Warn(err)
	} else {
		if err := os.RemoveAll(md5Path); err != nil {
			logger.Warn(fmt.Sprintf("failed to clean out %v%s file: %v", filepath.Base(path), Md5Extension, md5Path))
		}

		if err := os.RemoveAll(path); err != nil {
			logger.Warn(fmt.Sprintf("failed to clean out the destination directory: %v", path))
		}
	}
}

/*
get cache from shared file server to local
param - src         		source directory
param - dest				destination directory
param -	command				get
return - nil / error   		success - return nil; error - return error description
*/
func getCache(src, dest, command string) error {
	var (
		cwd, msg, srcZipPath, destPath, compressFormat string
	)
	logger.Info("get cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("directory [%v] check failed, do file check, command: %v", src, command)
		logger.Warn(msg)
	}

	if err != nil {
		info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatTarZst))
		if err != nil {
			msg = fmt.Sprintf("file %v not found, command: %v", fmt.Sprintf("%s%s", src, CompressFormatTarZst), command)
			logger.Warn(msg)

			// backward-compatibility to look for .zip file if .tar.zst is missing
			info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatZip))
			if err != nil {
				return logger.Error(fmt.Errorf("file %v not found, command: %v", fmt.Sprintf("%s%s", src, CompressFormatZip), command))
			}
		}
	}

	if info.IsDir() {
		srcZipPath = fmt.Sprintf("%s%s", filepath.Join(src, filepath.Base(src)), CompressFormatTarZst)
		destPath = dest
	} else {
		srcZipPath = fmt.Sprintf("%s%s", filepath.Join(filepath.Dir(src), filepath.Base(src)), CompressFormatTarZst)
		destPath = filepath.Dir(dest)
	}
	_, err = os.Lstat(srcZipPath)
	if err != nil {
		// backward-compatibility to look for .zip file if .tar.zst is missing
		if info.IsDir() {
			srcZipPath = fmt.Sprintf("%s%s", filepath.Join(src, filepath.Base(src)), CompressFormatZip)
			destPath = dest
		} else {
			srcZipPath = fmt.Sprintf("%s%s", filepath.Join(filepath.Dir(src), filepath.Base(src)), CompressFormatZip)
			destPath = filepath.Dir(dest)
		}
		compressFormat = CompressFormatZip
	} else {
		compressFormat = CompressFormatTarZst
	}

	switch compressFormat {
	case CompressFormatTarZst:
		// zstd route
		// check if .tar.zst file exist
		_, err = os.Lstat(srcZipPath)
		if err == nil {
			// if .tar.zst exist then
			cwd, err = os.Getwd()
			if err != nil {
				return logger.Error(err)
			}
			_ = os.MkdirAll(destPath, DefaultFilePermission)
			if err = acquireLock(srcZipPath, true); err == nil {
				if ZstdCli {
					cmd := fmt.Sprintf("cd %s && %s -cd -T0 %d %s | tar xf - || true; cd %s", destPath, getZstdBinary(), CompressionLevel, srcZipPath, cwd)
					err = executeCommand(cmd)
				} else {
					err = Decompress(srcZipPath, destPath)
				}
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("read failed, %v", err)
			}
		}

	default:
		_ = os.MkdirAll(filepath.Dir(destPath), DefaultFilePermission)

		targetZipPath := fmt.Sprintf("%s%s", dest, CompressFormatZip)
		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return logger.Error(err)
		}
		// destination is relative without subdirectories, unzip in SD Source Directory
		filePath := dest
		dest, _ = filepath.Split(filePath)
		if !strings.HasPrefix(filePath, "/") {
			wd, _ := os.Getwd()
			dest = filepath.Join(wd, dest)
		}
		_, err = Unzip(targetZipPath, dest)
		if err != nil {
			logger.Warn(fmt.Sprintf("could not unzip file %s", src))
		}
		defer os.RemoveAll(targetZipPath)

		if info.IsDir() {
			defer os.RemoveAll(filepath.Join(dest, fmt.Sprintf("%s%s", filepath.Base(dest), Md5Extension)))
		}
	}
	fmt.Println("get cache SUCCESS")
	logger.Info("get cache complete")

	return nil
}

/*
store cache in shared file server
param - src         		source directory
param - dest			destination directory
param -	command			set
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   		success - return nil; error - return error description
*/
func setCache(src, dest, command string, cacheMaxSizeInMB int64) error {
	var (
		msg, md5Path, destPath, destBase, srcPath, srcFile, cwd string
		err                                                     error
	)

	info, err := os.Lstat(src)
	if err != nil {
		return logger.Error(fmt.Errorf("%v, source path not found for command %v", err, command))
	}
	destBase = filepath.Base(dest) // get file name
	destPath = dest                // cache path + path from cache spec
	srcPath = src                  // path from cache spec
	srcFile = "."                  // assume path from cache spec is directory
	if !info.IsDir() {             // if path from cache spec is file
		destPath = filepath.Dir(dest)
		srcPath = filepath.Dir(src)
		srcFile = filepath.Base(src)
	}

	fInfos, newMd5, sizeInBytes := getMetadataInfo(src)
	if cacheMaxSizeInMB > 0 {
		cacheMaxSizeInBytes := cacheMaxSizeInMB << (10 * 2) // MB to Bytes
		fmt.Printf("size: %v B\n", sizeInBytes)
		if sizeInBytes > cacheMaxSizeInBytes {
			return logger.Error(fmt.Errorf("source directory size %v B is more than allowed max limit %v B", sizeInBytes, cacheMaxSizeInBytes))
		}
		logger.Info(fmt.Sprintf("source directory size %vB, allowed max limit %vB", sizeInBytes, cacheMaxSizeInBytes))
	}

	if compareMd5(newMd5, destPath, destBase) {
		logger.Warn(fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		return nil
	}

	targetPath := fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatTarZst)
	cwd, err = os.Getwd()
	if err != nil {
		return logger.Error(err)
	}
	_ = os.MkdirAll(destPath, DefaultFilePermission)

	if ZstdCli {
		if err = acquireLock(targetPath, false); err == nil {
			cmd := fmt.Sprintf("cd %s && tar -c %s | %s -T0 %d > %s || true; cd %s", srcPath, srcFile, getZstdBinary(), CompressionLevel, targetPath, cwd)
			err = executeCommand(cmd)
			if err != nil {
				msg = fmt.Sprintf("failed to compress files from %v", src)
				logger.Warn(msg)
			}
			_ = os.Chmod(destPath, DefaultFilePermission)
			_ = os.Chmod(targetPath, DefaultFilePermission)
			releaseLock(targetPath)
		} else {
			return logger.Error(fmt.Errorf("unable to acquire lock on file: %v, error: %v", targetPath, err))
		}
	} else {
		if err = acquireLock(targetPath, false); err == nil {
			err = Compress(srcPath, targetPath, fInfos)
			_ = os.Chmod(destPath, DefaultFilePermission)
			releaseLock(targetPath)
			if err != nil {
				return logger.Error(err)
			}
		} else {
			return logger.Error(err)
		}
	}
	// remove zip file if available
	targetPath = fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatZip)
	defer os.RemoveAll(targetPath)

	md5Path = filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, Md5Extension))
	if err = acquireLock(md5Path, false); err == nil {
		writeMd5(md5Path, newMd5)
		releaseLock(md5Path)
	} else {
		return logger.Error(err)
	}
	return nil
}

/*
cache directories and files to/from shared storage
param - command         	set, get or remove
param - cacheScope     		pipeline, event, job
param -	src     		source directory
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, src string, cacheMaxSizeInMB int64) error {
	var (
		info os.FileInfo
		err  error
	)

	homeDir, _ := os.UserHomeDir()
	baseCacheDir := ""
	command = strings.ToLower(strings.TrimSpace(command))
	cacheScope = strings.ToLower(strings.TrimSpace(cacheScope))

	if command != "set" && command != "get" && command != "remove" {
		return logger.Error(fmt.Errorf("%v, command: %v is not expected", err, command))
	}

	if cacheScope == "" {
		return logger.Error(fmt.Errorf("%v, cache scope %v empty", err, cacheScope))
	}

	switch cacheScope {
	case "pipeline":
		baseCacheDir = os.Getenv("SD_PIPELINE_CACHE_DIR")
	case "event":
		baseCacheDir = os.Getenv("SD_EVENT_CACHE_DIR")
	case "job":
		baseCacheDir = os.Getenv("SD_JOB_CACHE_DIR")
	}

	if strings.HasPrefix(baseCacheDir, "~/") {
		baseCacheDir = filepath.Join(homeDir, strings.TrimPrefix(baseCacheDir, "~/"))
	}
	if strings.HasPrefix(src, "~/") {
		src = filepath.Join(homeDir, strings.TrimPrefix(src, "~/"))
	}
	src = filepath.Clean(src)
	if strings.HasPrefix(src, "../") {
		if src, err = filepath.Abs(src); err != nil {
			return logger.Error(fmt.Errorf("%v in src path %v, command: %v", err, src, command))
		}
	}
	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		return logger.Error(fmt.Errorf("%v in path %v, command: %v", err, baseCacheDir, command))
	}

	if _, err := os.Lstat(baseCacheDir); err != nil {
		return logger.Error(fmt.Errorf("%v, cache path %s not found", err, baseCacheDir))
	}

	cache := filepath.Join(baseCacheDir, src)
	dest := cache

	switch command {
	case "set":
		fmt.Printf("set cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = setCache(src, dest, command, cacheMaxSizeInMB); err != nil {
			return logger.Error(fmt.Errorf("set cache FAILED"))
		}
		fmt.Println("set cache SUCCESS")
	case "get":
		dest = src
		src = cache
		fmt.Printf("get cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = getCache(src, dest, command); err != nil {
			logger.Warn(fmt.Sprintf("get cache FAILED"))
		}
	case "remove":
		fmt.Printf("remove cache -> {scope: %v, path: %v} \n", cacheScope, src)
		info, err = os.Lstat(dest)
		destBase := filepath.Base(dest)
		destPath := dest

		if err != nil {
			logger.Warn(err)
		} else {
			if !info.IsDir() {
				destPath = filepath.Dir(dest)
				destBase = filepath.Base(dest)
			}

			removeCacheDirectory(dest, filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, Md5Extension)))
		}
		fmt.Println("remove cache SUCCESS")
	}
	return nil
}
