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
		filepath.Walk(path, readSize)
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

	logger.Log(logger.LOGLEVEL_INFO, "", "start md5 check")
	oldMd5FilePath = filepath.Join(dest, fmt.Sprintf("%s%s", destBase, ".md5"))
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		msg = fmt.Sprintf("%v, not able to get %s.md5 from: %s", err, destBase, dest)
		logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = GenerateMd5(src); err != nil {
		msg = fmt.Sprintf("not able to generate md5 for directory: %s", src)
		logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, msg)
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
		logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("error: %v\n", err))
	} else {
		if err := os.RemoveAll(md5Path); err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("failed to clean out %v.md5 file: %v", filepath.Base(path), md5Path))
		}

		if err := os.RemoveAll(path); err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("failed to clean out the destination directory: %v", path))
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

	logger.Log(logger.LOGLEVEL_INFO, "", "", "get cache")
	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("directory [%v] check failed, do file check %v", src, command)
		logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
	}

	if compress {
		logger.Log(logger.LOGLEVEL_INFO, "", "", "zip enabled")

		if err != nil {
			info, err = os.Lstat(fmt.Sprintf("%s%s", src, ".zip"))
			if err != nil {
				msg = fmt.Sprintf("file check failed, for file %v, command %v", fmt.Sprintf("%s%s", src, ".zip"), command)
				return logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, msg)
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
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
		}
		// destination is relative without subdirectories, unzip in SD Source Directory
		filePath := dest
		dest, _ := filepath.Split(filePath)
		if !strings.HasPrefix(filePath, "/") {
			wd, _ := os.Getwd()
			dest = filepath.Join(wd, dest)
		}
		_, err = Unzip(targetZipPath, dest)
		if err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_ZIP, fmt.Sprintf("could not unzip file %s", src))
		}

		defer os.RemoveAll(targetZipPath)
	} else {
		logger.Log(logger.LOGLEVEL_INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
		}
	}
	if info.IsDir() {
		defer os.RemoveAll(filepath.Join(dest, fmt.Sprintf("%s%s", filepath.Base(dest), ".md5")))
	}
	return logger.Log(logger.LOGLEVEL_INFO, "", "", "get cache complete")
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

	info, err := os.Lstat(src)
	if err != nil {
		msg = fmt.Sprintf("%v, source path not found for command %v", err, command)
		return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_FILE, msg)
	}
	destBase = filepath.Base(dest)
	destPath = dest
	if !info.IsDir() {
		destPath = filepath.Dir(dest)
	}

	if cacheMaxSizeInMB > 0 {
		sizeInMB := int64(float64(getSizeInBytes(src)) * 0.000001)
		fmt.Printf("size: %vMB\n", sizeInMB)
		if sizeInMB > cacheMaxSizeInMB {
			msg = fmt.Sprintf("source directory size %vMB is more than allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB)
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_MAXSIZELIMIT, msg)
		}
		logger.Log(logger.LOGLEVEL_INFO, "", fmt.Sprintf("source directory size %vMB, allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB))
	}

	logger.Log(logger.LOGLEVEL_INFO, "", fmt.Sprintf("md5Check %v", md5Check))
	if md5Check {
		logger.Log(logger.LOGLEVEL_INFO, "", "starting md5Check")
		md5Json, md5Status = checkMd5(src, destPath, destBase)
		if md5Status {
			return logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_FILE, fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		}
		logger.Log(logger.LOGLEVEL_INFO, "", "md5Check complete")
	}
	removeCacheDirectory(dest, filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5")))

	if compress {
		logger.Log(logger.LOGLEVEL_INFO, "", "zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)

		targetZipPath = fmt.Sprintf("%s.zip", filepath.Join(destPath, destBase))

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
	} else {
		logger.Log(logger.LOGLEVEL_INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", logger.ERRTYPE_COPY, err)
		}
	}
	logger.Log(logger.LOGLEVEL_INFO, "", "set cache complete")
	if md5Check {
		md5Path = filepath.Join(destPath, fmt.Sprintf("%s%s", destBase, ".md5"))
		jsonFile, err := os.Create(md5Path)
		if err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, fmt.Sprintf("not able to create %v file", md5Path))
		} else {
			defer jsonFile.Close()
			if b, err = jsonFile.Write(md5Json); err != nil {
				logger.Log(logger.LOGLEVEL_WARN, "", logger.ERRTYPE_MD5, fmt.Sprintf("failed to write %v.md5 file to destination %v", destBase, destPath))
			} else {
				jsonFile.Sync()
				logger.Log(logger.LOGLEVEL_INFO, "", "", fmt.Sprintf("wrote %d bytes of %v.md5 file to destination %v", b, destBase, destPath))
			}
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
	if strings.HasPrefix(srcDir, "~/") {
		srcDir = filepath.Join(homeDir, strings.TrimPrefix(srcDir, "~/"))
	}
	srcDir = filepath.Clean(srcDir)
	if strings.HasPrefix(srcDir, "../") {
		if srcDir, err = filepath.Abs(srcDir); err != nil {
			msg = fmt.Sprintf("%v in src path %v, command: %v", err, srcDir, command)
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

	cacheDir := filepath.Join(baseCacheDir, srcDir)
	src := srcDir
	dest := cacheDir

	switch command {
	case "set":
		fmt.Printf("set cache, scope: %v, path: %v\n", cacheScope, srcDir)
		if err = setCache(src, dest, command, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return logger.Log(logger.LOGLEVEL_ERROR, "", "", fmt.Sprintf("set cache FAILED"))
		}
		fmt.Println("set cache completed")
	case "get":
		src = cacheDir
		dest = srcDir
		fmt.Printf("get cache, scope: %v, path: %v\n", cacheScope, srcDir)
		if err = getCache(src, dest, command, compress); err != nil {
			logger.Log(logger.LOGLEVEL_WARN, "", "", fmt.Sprintf("get cache FAILED"))
		}
		fmt.Println("get cache completed")
	case "remove":
		fmt.Printf("remove cache, scope: %v, directory: %v\n", cacheScope, srcDir)
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
		fmt.Println("remove cache completed")
	}
	return nil
}
