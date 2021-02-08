package sdstore

import (
	"encoding/json"
	"fmt"
	"github.com/otiai10/copy"
	"github.com/screwdriver-cd/store-cli/logger"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const CompressFormatTarZst = ".tar.zst"
const CompressFormatZip = ".zip"

// taken and modified from https://stackoverflow.com/questions/32482673/how-to-get-directory-total-size
/*
get directory size recursive in bytes
param 	- path			directory path
return 	- size in bytes
*/
func getSizeInBytes(path string) int64 {
	sizes := make(chan int64)
	readSize := func(path string, file os.FileInfo, err error) error {
		if err != nil || file == nil {
			return nil
		}
		if !file.IsDir() {
			sizes <- file.Size()
		}
		return nil
	}

	go func() {
		_ = filepath.Walk(path, readSize)
		close(sizes)
	}()

	size := int64(0)
	for s := range sizes {
		size += s
	}

	return size
}

/*
compare md5 for source and destination directories
param - src         		source directory
param - dest			all directory but the last element of path
param - destBase		last element of destination directory
return - bytearray / bool   	return md5 byte array, bool - true (md5 same) / false (md5 changed)
*/
func checkMd5(src, dest, destBase string) ([]byte, bool) {
	var msg, oldMd5FilePath string
	var oldMd5 map[string]string
	var newMd5 map[string]string

	_ = logger.Log(logger.LOGLEVEL_INFO, "", "start md5 check")
	oldMd5FilePath = filepath.Join(dest, fmt.Sprintf("%s%s", destBase, ".md5"))
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		msg = fmt.Sprintf("%v, not able to get %s.md5 from: %s", err, destBase, dest)
		_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = GenerateMd5(src); err != nil {
		msg = fmt.Sprintf("not able to generate md5 for directory: %s", src)
		_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, msg)
	}
	md5Json, _ := json.Marshal(newMd5)

	if reflect.DeepEqual(oldMd5, newMd5) {
		return md5Json, true
	} else {
		return md5Json, false
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
		_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("error: %v\n", err))
	} else {
		if err := os.RemoveAll(md5Path); err != nil {
			_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("failed to clean out %v.md5 file: %v", filepath.Base(path), md5Path))
		}

		if err := os.RemoveAll(path); err != nil {
			_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("failed to clean out the destination directory: %v", path))
		}
	}
}

/*
get cache from shared file server to local
param - src         		source directory
param - dest				destination directory
param -	command				get
param - compressFormat		.tar.zst / .zip
param - compress			get compressed cache
return - nil / error   		success - return nil; error - return error description
*/
func getCache(src, dest, command, compressFormat string, compress bool) error {
	var msg, srcZipPath, destPath string

	_ = logger.Log(logger.LOGLEVEL_INFO, "", "", "get cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("directory [%v] check failed, do file check %v", src, command)
		_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
	}

	if compress {
		if err != nil {
			switch compressFormat {
			case CompressFormatTarZst:
				info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatTarZst))
				if err != nil {
					msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, CompressFormatTarZst), command)
					_ = logger.Log(logger.LOGLEVEL_INFO, "", logger.ERRTYPE_FILE, msg)

					// backward-compatibility to look for .zip file if .tar.zst is missing
					info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatZip))
					if err != nil {
						msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, CompressFormatZip), command)
						return logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
					}
				}
			default:
				info, err = os.Lstat(fmt.Sprintf("%s%s", src, CompressFormatZip))
				if err != nil {
					msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, CompressFormatZip), command)
					return logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
				}
			}
		}

		switch compressFormat {
		case CompressFormatTarZst:
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
		default:
			if info.IsDir() {
				srcZipPath = fmt.Sprintf("%s%s", filepath.Join(src, filepath.Base(src)), CompressFormatZip)
				destPath = dest
			} else {
				srcZipPath = fmt.Sprintf("%s%s", filepath.Join(filepath.Dir(src), filepath.Base(src)), CompressFormatZip)
				destPath = filepath.Dir(dest)
			}
			compressFormat = CompressFormatZip
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
					return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_ZIP, err)
				}
				_ = os.MkdirAll(destPath, 0777)
				cmd := fmt.Sprintf("cd %s && zstd -cd -T0 --fast %s | tar xf - || true; cd %s", destPath, srcZipPath, cwd)
				err = ExecuteCommand(cmd)
				if err != nil {
					msg = fmt.Sprintf("failed to compress files from %v", src)
					return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_ZIP, msg)
				}
				_ = os.Chmod(destPath, 0777)
			}

		default:
			_ = os.MkdirAll(filepath.Dir(destPath), 0777)

			targetZipPath := fmt.Sprintf("%s%s", dest, CompressFormatZip)
			if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
				return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
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
				_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_ZIP, fmt.Sprintf("could not unzip file %s", src))
			}
			defer os.RemoveAll(targetZipPath)

			if info.IsDir() {
				defer os.RemoveAll(filepath.Join(dest, fmt.Sprintf("%s%s", filepath.Base(dest), ".md5")))
			}
		}
	} else {
		_ = logger.Log(logger.LOGLEVEL_INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
		}
	}

	fmt.Println("get cache SUCCESS")
	return logger.Log(logger.LOGLEVEL_INFO, "", "", "get cache complete")
}

/*
store cache in shared file server
param - src         		source directory
param - dest			destination directory
param -	command			set
param - compressFormat  .tar.zst / .zip
param - compress		compress and store cache
param - md5Check		compare md5 and store cache
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   		success - return nil; error - return error description
*/
func setCache(src, dest, command, compressFormat string, compress, md5Check bool, cacheMaxSizeInMB int64) error {
	var msg, md5Path, destPath, destBase, srcPath, srcFile, cwd string
	var md5Status bool
	var b int
	var md5Json []byte
	var err error
	var jsonFile *os.File

	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("%v, source path not found for command %v", err, command)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_FILE, msg)
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

	if cacheMaxSizeInMB > 0 {
		sizeInBytes := getSizeInBytes(src)
		cacheMaxSizeInBytes := cacheMaxSizeInMB << (10 * 2) // MB to Bytes
		fmt.Printf("size: %v B\n", sizeInBytes)
		if sizeInBytes > cacheMaxSizeInBytes {
			msg = fmt.Sprintf("source directory size %v B is more than allowed max limit %v B", sizeInBytes, cacheMaxSizeInBytes)
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_MAXSIZELIMIT, msg)
		}
		_ = logger.Log(logger.LOGLEVEL_INFO, "", fmt.Sprintf("source directory size %vB, allowed max limit %vB", sizeInBytes, cacheMaxSizeInBytes))
	}

	_ = logger.Log(logger.LOGLEVEL_INFO, "", fmt.Sprintf("md5Check %v", md5Check))
	if md5Check {
		md5Json, md5Status = checkMd5(src, destPath, destBase)
		if md5Status {
			return logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		}
	}

	if compress {
		switch compressFormat {
		case CompressFormatTarZst:
			targetPath := fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatTarZst)
			cwd, err = os.Getwd()
			if err != nil {
				return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_ZIP, err)
			}
			_ = os.MkdirAll(destPath, 0777)
			cmd := fmt.Sprintf("cd %s && tar -c %s | zstd -T0 --fast > %s || true; cd %s", srcPath, srcFile, targetPath, cwd)
			err = ExecuteCommand(cmd)
			if err != nil {
				msg = fmt.Sprintf("failed to compress files from %v", src)
				return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_ZIP, msg)
			}
			_ = os.Chmod(destPath, 0777)

		default:
			_ = logger.Log(logger.LOGLEVEL_INFO, "", "zip enabled")
			srcZipPath := fmt.Sprintf("%s%s", src, CompressFormatZip)
			targetZipPath := fmt.Sprintf("%s%s", filepath.Join(destPath, destBase), CompressFormatZip)
			err = Zip(src, srcZipPath)
			if err != nil {
				msg = fmt.Sprintf("failed to zip files from %v to %v", src, srcZipPath)
				return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_ZIP, msg)
			}
			_ = os.MkdirAll(destPath, 0777)
			if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
				return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
			}
			defer os.RemoveAll(srcZipPath)
		}
	} else {
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
		}
	}

	if md5Check {
		md5Path = filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5"))
		jsonFile, err = os.OpenFile(md5Path, os.O_CREATE|os.O_WRONLY, 0777)
		if err != nil {
			_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, fmt.Sprintf("not able to create %v file", md5Path))
		} else {
			defer jsonFile.Close()
			if b, err = jsonFile.Write(md5Json); err != nil {
				_ = logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, fmt.Sprintf("failed to write %v.md5 file to destination %v", destBase, destPath))
			} else {
				jsonFile.Sync()
				_ = logger.Log(logger.LOGLEVEL_INFO, "", "", fmt.Sprintf("wrote %d bytes of %v.md5 file to destination %v", b, destBase, destPath))
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
param - compressFormat  .tar.zst / .zip
param - compress		compress and store cache
param - md5Check		compare md5 and store cache
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, src, compressFormat string, compress, md5Check bool, cacheMaxSizeInMB int64) error {
	var msg string
	var err error

	homeDir, _ := os.UserHomeDir()
	baseCacheDir := ""
	command = strings.ToLower(strings.TrimSpace(command))
	cacheScope = strings.ToLower(strings.TrimSpace(cacheScope))

	if command != "set" && command != "get" && command != "remove" {
		msg = fmt.Sprintf("%v, command: %v is not expected", err, command)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COMMAND, msg)
	}

	if cacheScope == "" {
		msg = fmt.Sprintf("%v, cache scope %v empty", err, cacheScope)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_SCOPE, msg)
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
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_FILE, msg)
		}
	}
	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v in path %v, command: %v", err, baseCacheDir, command)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_FILE, msg)
	}

	if _, err := os.Lstat(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v, cache path %s not found", err, baseCacheDir)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_FILE, msg)
	}

	cache := filepath.Join(baseCacheDir, src)
	dest := cache

	fmt.Println("Compress Format: ", compressFormat)
	switch command {
	case "set":
		fmt.Printf("set cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = setCache(src, dest, command, compressFormat, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", "", fmt.Sprintf("set cache FAILED"))
		}
		fmt.Println("set cache SUCCESS")
	case "get":
		dest = src
		src = cache
		fmt.Printf("get cache -> {scope: %v, path: %v} \n", cacheScope, src)
		if err = getCache(src, dest, command, compressFormat, compress); err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", "", fmt.Sprintf("get cache FAILED"))
		}
	case "remove":
		fmt.Printf("remove cache -> {scope: %v, path: %v} \n", cacheScope, src)
		info, err := os.Lstat(dest)
		destBase := filepath.Base(dest)
		destPath := dest

		if err != nil {
			fmt.Printf("error: %v\n", err)
			logger.Log(logger.LOGLEVEL_WARN, "", "", fmt.Sprintf("error: %v", err))
		} else {
			if !info.IsDir() {
				destPath = filepath.Dir(dest)
				destBase = filepath.Base(dest)
			}

			removeCacheDirectory(dest, filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5")))
		}
		fmt.Println("remove cache SUCCESS")
	}
	return nil
}
