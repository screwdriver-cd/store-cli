package sdstore

import (
	"encoding/json"
	"fmt"
	"github.com/otiai10/copy"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/screwdriver-cd/store-cli/logger"
)

/*
write log
param - level         		log level
param - errType
param - msg			error msg
return - error / nil		INFO / WARN - nil; ERROR - return error msg
*/
func writeLog(level, errType string, msg ...interface{}) error {
	switch level {
	case logger.WARN:
		msg = append([]interface{}{"ignore warning, "}, msg...)
		logger.Log(level, "cache2disk.go", errType, msg...)
		return nil
	case logger.ERROR:
		logger.Log(level, "cache2disk.go", errType, msg...)
		return fmt.Errorf(fmt.Sprintf("%v", msg...))
	default:
		logger.Log(level, "cache2disk.go", errType, msg...)
		return nil
	}
}

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

	_ = writeLog(logger.INFO, "", "start md5 check")
	oldMd5FilePath = filepath.Join(dest, fmt.Sprintf("%s%s", destBase, ".md5"))
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		msg = fmt.Sprintf("%v, not able to get %v.md5 from: %v", err, destBase, dest)
		_ = writeLog(logger.WARN, logger.FILE, msg)
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = MD5All(src); err != nil {
		msg = fmt.Sprintf("not able to generate md5 for directory: %v", src)
		_ = writeLog(logger.WARN, logger.MD5, msg)
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
func removeCacheDirectory(path, md5Path, command string) {
	_, err := os.Lstat(path)

	if err != nil {
		_ = writeLog(logger.WARN, logger.FILE, fmt.Sprintf("cache directory does not exist: %v", path))
	} else {
		if err := os.RemoveAll(md5Path); err != nil {
			_ = writeLog(logger.WARN, logger.FILE, fmt.Sprintf("failed to clean out %v.md5 file: %v", filepath.Base(path), md5Path))
		}

		if err := os.RemoveAll(path); err != nil {
			_ = writeLog(logger.WARN, logger.FILE, fmt.Sprintf("failed to clean out the destination directory: %v", path))
		}
	}
}

/*
get cache from shared file server to local
param - src         		source directory
param - dest			destination directory
param -	command			get
param - compress		get compressed cache
return - nil / error   		success - return nil; error - return error description
*/
func getCache(src, dest, command string, compress bool) error {
	var msg, srcZipPath string

	_ = writeLog(logger.INFO, "", "get cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("directory [%v] check failed, do file check %v", src, command)
		writeLog(logger.WARN, logger.FILE, msg)
	}

	if compress {
		writeLog(logger.INFO, "zip enabled")

		if err != nil {
			info, err = os.Lstat(fmt.Sprintf("%s%s", src, ".zip"))
			if err != nil {
				msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, ".zip"), command)
				return writeLog(logger.WARN, logger.FILE, msg)
			}
		}

		if info.IsDir() {
			srcZipPath = fmt.Sprintf("%s.zip", filepath.Join(src, filepath.Base(src)))
			_ = os.MkdirAll(dest, 0777)
		} else {
			srcZipPath = fmt.Sprintf("%s.zip", filepath.Join(filepath.Dir(src), filepath.Base(src)))
			_ = os.MkdirAll(filepath.Dir(dest), 0777)
		}

		targetZipPath := fmt.Sprintf("%s.zip", dest)

		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return writeLog(logger.ERROR, logger.COPY, err)
		}
		_, err = Unzip(targetZipPath, filepath.Dir(dest))
		if err != nil {
			_ = writeLog(logger.WARN, logger.ZIP, fmt.Sprintf("could not unzip file %s", src))
		}

		defer os.RemoveAll(targetZipPath)
	} else {
		_ = writeLog(logger.INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return writeLog(logger.ERROR, logger.COPY, err)
		}
	}
	if info.IsDir() {
		defer os.RemoveAll(filepath.Join(dest, fmt.Sprintf("%s%s", filepath.Base(dest), ".md5")))
	}
	return writeLog(logger.INFO, "", "get cache complete")
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
	var msg, md5Path, targetZipPath, destPath, destBase string
	var md5Status bool
	var b int
	var md5Json []byte

	fmt.Println("set cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("%v, source path not found for command %v", err, command)
		return writeLog(logger.ERROR, logger.FILE, msg)
	}
	destBase = filepath.Base(dest)
	destPath = dest
	if !info.IsDir() {
		destPath = filepath.Dir(dest)
	}

	if cacheMaxSizeInMB > 0 {
		sizeInMB := int64(float64(getSizeInBytes(src)) * 0.000001)
		if sizeInMB > cacheMaxSizeInMB {
			msg = fmt.Sprintf("source directory size %vMB is more than allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB)
			return writeLog(logger.ERROR, logger.MAXSIZELIMIT, msg)
		}
		_ = writeLog(logger.INFO, "", fmt.Sprintf("source directory size %vMB, allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB))
	}

	_ = writeLog(logger.INFO, "", fmt.Sprintf("md5Check %v", md5Check))
	if md5Check {
		_ = writeLog(logger.INFO, "", "starting md5Check")
		md5Json, md5Status = checkMd5(src, destPath, destBase)
		if md5Status {
			return writeLog(logger.WARN, logger.FILE, fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		}
		_ = writeLog(logger.INFO, "", "md5Check complete")
	}
	removeCacheDirectory(dest, filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5")), command)

	if compress {
		_ = writeLog(logger.INFO, "", "zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)

		targetZipPath = fmt.Sprintf("%s.zip", filepath.Join(destPath, destBase))

		err = Zip(src, srcZipPath)
		if err != nil {
			msg = fmt.Sprintf("%v, failed to zip files from %v to %v", err, src, srcZipPath)
			return writeLog(logger.ERROR, logger.ZIP, msg)
		}

		_ = os.MkdirAll(destPath, 0777)

		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return writeLog(logger.ERROR, logger.COPY, err)
		}
		defer os.RemoveAll(srcZipPath)
	} else {
		_ = writeLog(logger.INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return writeLog(logger.ERROR, logger.COPY, err)
		}
	}
	_ = writeLog(logger.INFO, "", "set cache complete")
	if md5Check {
		md5Path = filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5"))
		jsonFile, err := os.Create(md5Path)
		if err != nil {
			_ = writeLog(logger.WARN, logger.MD5, fmt.Sprintf("not able to create %v file", md5Path))
		}
		defer jsonFile.Close()
		if b, err = jsonFile.Write(md5Json); err != nil {
			_ = writeLog(logger.WARN, logger.MD5, fmt.Sprintf("failed to write %v.md5 file to destination %v", destBase, destPath))
		} else {
			_ = jsonFile.Sync()
			_ = writeLog(logger.INFO, "", fmt.Sprintf("wrote %d bytes of %v.md5 file to destination %v", b, destBase, destPath))
		}
	}
	return nil
}

/*
cache directories and files to/from shared storage
param - command         	set, get or remove
param - cacheScope     		pipeline, event, job
param -	srcDir     		source directory
param - compress		compress and store cache
param - md5Check		compare md5 and store cache
param - cacheMaxSizeInMB	max cache size limit allowed in MB
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, srcDir string, compress, md5Check bool, cacheMaxSizeInMB int64) error {
	var msg string
	var err error

	homeDir, _ := os.UserHomeDir()
	baseCacheDir := ""
	command = strings.ToLower(strings.TrimSpace(command))
	cacheScope = strings.ToLower(strings.TrimSpace(cacheScope))

	if command != "set" && command != "get" && command != "remove" {
		msg = fmt.Sprintf("%v, command: %v is not expected", err, command)
		return writeLog(logger.ERROR, logger.COMMAND, msg)
	}

	if cacheScope == "" {
		msg = fmt.Sprintf("%v, cache scope %v empty", err, cacheScope)
		return writeLog(logger.ERROR, logger.SCOPE, msg)
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

	if strings.HasPrefix(srcDir, "~/") {
		srcDir = filepath.Join(homeDir, strings.TrimPrefix(srcDir, "~/"))
	}

	if srcDir, err = filepath.Abs(srcDir); err != nil {
		msg = fmt.Sprintf("%v in src path %v, command: %v", err, srcDir, command)
		return writeLog(logger.ERROR, logger.FILE, msg)
	}

	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v in path %v, command: %v", err, baseCacheDir, command)
		return writeLog(logger.ERROR, logger.FILE, msg)
	}

	if _, err := os.Lstat(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v, cache path %s not found", err, baseCacheDir)
		return writeLog(logger.ERROR, logger.FILE, msg)
	}

	cacheDir := filepath.Join(baseCacheDir, srcDir)
	src := srcDir
	dest := cacheDir

	switch command {
	case "set":
		if err = setCache(src, dest, command, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return writeLog(logger.ERROR, "", fmt.Sprintf("set cache FAILED"))
		}
		fmt.Println("set cache complete")
	case "get":
		src = cacheDir
		dest = srcDir
		if err = getCache(src, dest, command, compress); err != nil {
			_ = writeLog(logger.WARN, "", fmt.Sprintf("get cache FAILED"))
		}
		fmt.Println("get cache complete")
	case "remove":
		info, err := os.Lstat(dest)
		destBase := filepath.Base(dest)
		destPath := dest

		if err != nil {
			_ = writeLog(logger.WARN, "", fmt.Sprintf("path %v does not exist", dest))
		} else {
			if !info.IsDir() {
				destPath = filepath.Dir(dest)
				destBase = filepath.Base(dest)
			}

			removeCacheDirectory(dest, filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5")), command)
		}
		fmt.Println("remove cache complete")
	}
	return nil
}
