package sdstore

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/karrick/godirwalk"
	"github.com/otiai10/copy"
	"github.com/screwdriver-cd/store-cli/logger"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const CompressFormatTarZst = ".tar.zst"
const CompressFormatZip = ".zip"
const Md5Extension = ".md5"
const CompressionLevel = "-3" //	default compression level - 3 / possible values (1-19) or --fast

type FileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modtime"`
	Mode    string `json:"mode"`
}

// ExecCommand : os exec command
var ExecCommand = exec.Command

// ExecuteCommand : Execute shell commands
// return output => executing shell command succeeds
// return error => for any error
func ExecuteCommand(command string) error {
	_ = logger.Log(logger.LoglevelInfo, ZiphelperModule, "executeCommand", command)
	cmd := ExecCommand("sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil || strings.TrimSpace(stderr.String()) != "" {
		return logger.Log(logger.LoglevelError, "", "", fmt.Sprintf("error: %v, %v, out: %v", err, stderr.String(), stdout.String()))
	}
	_ = logger.Log(logger.LoglevelInfo, "", stdout.String())
	return nil
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

/*
Get metadata and return md5 and total size for the given path.
using file meta instead of calculating md5 for each file content, as its slower than storing the cache
param - path         		file / folder path
return - string / int64 	md5 for path / total size in bytes
*/
func getMetadataInfo(path string) (string, int64) {
	var fileInfos []*FileInfo

	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(filePath string, de *godirwalk.Dirent) error {
			if !de.IsDir() {
				file, err := os.Lstat(filePath)
				if err != nil || file == nil {
					return nil
				}
				fileInfos = append(fileInfos, &FileInfo{filePath, file.Size(), file.ModTime().UnixNano(), file.Mode().String()})
			}
			return nil
		},
		ErrorCallback: func(filePath string, err error) godirwalk.ErrorAction {
			_ = logger.Log(logger.LoglevelWarn, "getMetadataInfo", err.Error())
			return godirwalk.SkipNode
		},
		Unsorted:            false,
		AllowNonDirectory:   true,
		FollowSymbolicLinks: false,
	})

	size := int64(0)
	if err != nil {
		return "", size
	}

	for _, s := range fileInfos {
		size += s.Size
	}

	md5Json, _ := json.Marshal(fileInfos)
	return getMd5(md5Json), size
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

	_ = logger.Log(logger.LoglevelInfo, "", "start md5 check")
	oldMd5FilePath = filepath.Join(dest, fmt.Sprintf("%s%s", destBase, Md5Extension))
	oldMd5InBytes, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5InBytes = []byte("")
		msg = fmt.Sprintf("%v, not able to get %s%s from: %s", err, destBase, Md5Extension, dest)
		_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, msg)
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
		_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, fmt.Sprintf("error: %v\n", err))
	} else {
		if err := os.RemoveAll(md5Path); err != nil {
			_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, fmt.Sprintf("failed to clean out %v%s file: %v", filepath.Base(path), Md5Extension, md5Path))
		}

		if err := os.RemoveAll(path); err != nil {
			_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, fmt.Sprintf("failed to clean out the destination directory: %v", path))
		}
	}
}

/*
get cache from shared file server to local
param - src         		source directory
param - dest				destination directory
param -	command				get
param - compress			get compressed cache
return - nil / error   		success - return nil; error - return error description
*/
func getCache(src, dest, command string, compress bool) error {
	var msg, srcZipPath, destPath, compressFormat string

	_ = logger.Log(logger.LoglevelInfo, "", "", "get cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("directory [%v] check failed, do file check %v", src, command)
		_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, msg)
	}

	if compress {
		if err != nil {
			info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatTarZst))
			if err != nil {
				msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, CompressFormatTarZst), command)
				_ = logger.Log(logger.LoglevelInfo, "", logger.ErrtypeFile, msg)

				// backward-compatibility to look for .zip file if .tar.zst is missing
				info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatZip))
				if err != nil {
					msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, CompressFormatZip), command)
					return logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, msg)
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
			_, err := os.Lstat(srcZipPath)
			if err == nil {
				// if .tar.zst exist then
				cwd, err := os.Getwd()
				if err != nil {
					return logger.Log(logger.LoglevelError, "", logger.ErrtypeZip, err)
				}
				_ = os.MkdirAll(destPath, 0777)
				cmd := fmt.Sprintf("cd %s && %s -cd -T0 --fast %s | tar xf - || true; cd %s", destPath, getZstdBinary(), srcZipPath, cwd)
				err = ExecuteCommand(cmd)
				if err != nil {
					msg = fmt.Sprintf("error decompressing files from %v", src)
					_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeZip, msg)
				}
			}

		default:
			_ = os.MkdirAll(filepath.Dir(destPath), 0777)

			targetZipPath := fmt.Sprintf("%s%s", dest, CompressFormatZip)
			if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
				return logger.Log(logger.LoglevelError, "", logger.ErrtypeCopy, err)
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
				_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeZip, fmt.Sprintf("could not unzip file %s", src))
			}
			defer os.RemoveAll(targetZipPath)

			if info.IsDir() {
				defer os.RemoveAll(filepath.Join(dest, fmt.Sprintf("%s%s", filepath.Base(dest), Md5Extension)))
			}
		}
	} else {
		_ = logger.Log(logger.LoglevelInfo, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LoglevelError, "", logger.ErrtypeCopy, err)
		}
	}

	fmt.Println("get cache SUCCESS")
	return logger.Log(logger.LoglevelInfo, "", "", "get cache complete")
}

/*
store cache in shared file server
param - src         		source directory
param - dest			destination directory
param -	command			set
param - compress		compress and store cache
param - md5Check		compare md5 and store cache
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   		success - return nil; error - return error description
*/
func setCache(src, dest, command string, compress, md5Check bool, cacheMaxSizeInMB int64) error {
	var msg, md5Path, destPath, destBase, srcPath, srcFile, cwd string
	var b int
	var err error
	var md5File *os.File

	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("%v, source path not found for command %v", err, command)
		return logger.Log(logger.LoglevelError, "", logger.ErrtypeFile, msg)
	}
	destBase = filepath.Base(dest) // get file name
	destPath = dest                // cache path + path from cache spec
	srcPath = src                  // path from cache spec
	srcFile = "."                  // assume patch from cache spec is directory
	if !info.IsDir() {             // if path from cache spec is file
		destPath = filepath.Dir(dest)
		srcPath = filepath.Dir(src)
		srcFile = filepath.Base(src)
	}

	newMd5, sizeInBytes := getMetadataInfo(src)
	if cacheMaxSizeInMB > 0 {
		cacheMaxSizeInBytes := cacheMaxSizeInMB << (10 * 2) // MB to Bytes
		fmt.Printf("size: %v B\n", sizeInBytes)
		if sizeInBytes > cacheMaxSizeInBytes {
			msg = fmt.Sprintf("source directory size %v B is more than allowed max limit %v B", sizeInBytes, cacheMaxSizeInBytes)
			return logger.Log(logger.LoglevelError, "", logger.ErrtypeMaxsizelimit, msg)
		}
		_ = logger.Log(logger.LoglevelInfo, "", fmt.Sprintf("source directory size %vB, allowed max limit %vB", sizeInBytes, cacheMaxSizeInBytes))
	}

	_ = logger.Log(logger.LoglevelInfo, "", fmt.Sprintf("md5 check %v", md5Check))
	if md5Check {
		if compareMd5(newMd5, destPath, destBase) {
			return logger.Log(logger.LoglevelWarn, "", logger.ErrtypeFile, fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		}
	}

	if compress {
		targetPath := fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatTarZst)
		cwd, err = os.Getwd()
		if err != nil {
			return logger.Log(logger.LoglevelError, "", logger.ErrtypeZip, err)
		}
		_ = os.MkdirAll(destPath, 0777)
		cmd := fmt.Sprintf("cd %s && tar -c %s | %s -T0 %s > %s || true; cd %s", srcPath, srcFile, getZstdBinary(), CompressionLevel, targetPath, cwd)
		err = ExecuteCommand(cmd)
		if err != nil {
			msg = fmt.Sprintf("failed to compress files from %v", src)
			_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeZip, msg)
		}
		_ = os.Chmod(targetPath, 0777)
		_ = os.Chmod(destPath, 0777)

		// remove zip file if available
		targetPath = fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatZip)
		defer os.RemoveAll(targetPath)
	} else {
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LoglevelError, "", logger.ErrtypeCopy, err)
		}
	}

	if md5Check {
		md5Path = filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, Md5Extension))
		md5File, err = os.OpenFile(md5Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
		if err != nil {
			_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeMd5, fmt.Sprintf("not able to create %v file", md5Path))
		} else {
			defer md5File.Close()
			if b, err = md5File.WriteString(newMd5); err != nil {
				_ = logger.Log(logger.LoglevelWarn, "", logger.ErrtypeMd5, fmt.Sprintf("failed to write %v%s file to destination %v", destBase, Md5Extension, destPath))
			} else {
				_ = md5File.Sync()
				_ = logger.Log(logger.LoglevelInfo, "", "", fmt.Sprintf("wrote %d bytes of %v%s file to destination %v", b, destBase, Md5Extension, destPath))
			}
		}
	}
	return nil
}

/*
cache directories and files to/from shared storage
param - command         	set, get or remove
param - cacheScope     		pipeline, event, job
param -	src     		source directory
param - compress		compress and store cache
param - md5Check		compare md5 and store cache
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, src string, compress, md5Check bool, cacheMaxSizeInMB int64) error {
	var msg string
	var err error

	homeDir, _ := os.UserHomeDir()
	baseCacheDir := ""
	command = strings.ToLower(strings.TrimSpace(command))
	cacheScope = strings.ToLower(strings.TrimSpace(cacheScope))

	if command != "set" && command != "get" && command != "remove" {
		msg = fmt.Sprintf("%v, command: %v is not expected", err, command)
		return logger.Log(logger.LoglevelError, "", logger.ErrtypeCommand, msg)
	}

	if cacheScope == "" {
		msg = fmt.Sprintf("%v, cache scope %v empty", err, cacheScope)
		return logger.Log(logger.LoglevelError, "", logger.ErrtypeScope, msg)
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
			msg = fmt.Sprintf("%v in src path %v, command: %v", err, src, command)
			return logger.Log(logger.LoglevelError, "", logger.ErrtypeFile, msg)
		}
	}
	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v in path %v, command: %v", err, baseCacheDir, command)
		return logger.Log(logger.LoglevelError, "", logger.ErrtypeFile, msg)
	}

	if _, err := os.Lstat(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v, cache path %s not found", err, baseCacheDir)
		return logger.Log(logger.LoglevelError, "", logger.ErrtypeFile, msg)
	}

	cache := filepath.Join(baseCacheDir, src)
	dest := cache

	switch command {
	case "set":
		fmt.Printf("set cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = setCache(src, dest, command, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return logger.Log(logger.LoglevelError, "", "", fmt.Sprintf("set cache FAILED"))
		}
		fmt.Println("set cache SUCCESS")
	case "get":
		dest = src
		src = cache
		fmt.Printf("get cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = getCache(src, dest, command, compress); err != nil {
			_ = logger.Log(logger.LoglevelWarn, "", "", fmt.Sprintf("get cache FAILED"))
		}
	case "remove":
		fmt.Printf("remove cache -> {scope: %v, path: %v} \n", cacheScope, src)
		info, err := os.Lstat(dest)
		destBase := filepath.Base(dest)
		destPath := dest

		if err != nil {
			fmt.Printf("error: %v\n", err)
			_ = logger.Log(logger.LoglevelWarn, "", "", fmt.Sprintf("error: %v", err))
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
