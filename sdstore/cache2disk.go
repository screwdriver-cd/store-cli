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

	log "github.com/screwdriver-cd/store-cli/logger"
)

/*
write log
param - level         		log level
param - errType
param - msg			error msg
return - error / nil		INFO / WARN - nil; ERROR - return error msg
*/
func writeLog(level, errType string, msg ...interface{}) error {
	log.Write(level, "cache2disk.go", errType, msg...)

	if level == log.ERROR {
		return fmt.Errorf(fmt.Sprintf("%v", msg...))
	} else {
		return nil
	}
}

// taken and modified from https://stackoverflow.com/questions/32482673/how-to-get-directory-total-size
/*
get directory size recursive in bytes
param - path         		directory path
return - size in bytes
*/
func getDirSizeInBytes(path string) int64 {
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
param - dest			destination directory
return - bytearray / bool   	return md5 byte array, bool - true (md5 same) / false (md5 changed)
*/
func checkMd5(src, dest string) ([]byte, bool) {
	var msg string
	var oldMd5 map[string]string
	var newMd5 map[string]string

	fmt.Println("start md5 check")
	oldMd5FilePath := filepath.Join(filepath.Dir(dest), "md5.json")
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		msg = fmt.Sprintf("error: %v, not able to get md5.json from: %v", err, filepath.Dir(dest))
		_ = writeLog(log.WARN, log.FILE, msg)
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = MD5All(src); err != nil {
		msg = fmt.Sprintf("error: %v, not able to generate md5 for directory: %v", err, src)
		_ = writeLog(log.WARN, log.MD5, msg)
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
param -	command			set or remove
return - nothing
*/
func removeCacheDirectory(path, command string) {
	path = filepath.Dir(path)
	if err := os.RemoveAll(path); err != nil {
		_ = writeLog(log.WARN, log.FILE, fmt.Sprintf("error: %v, failed to clean out the destination directory: %v", err, path))
	}
	_ = writeLog(log.INFO, "", fmt.Sprintf("command: %v, cache directories %v removed", command, path))
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
	var msg string
	var err error

	if compress {
		if _, err = os.Stat(filepath.Dir(src)); err != nil {
			msg = fmt.Sprintf("skipping source path %v not found error for command %v, error: %v", src, command, err)
			return writeLog(log.WARN, log.FILE, msg)
		}
	} else {
		if _, err = os.Stat(src); err != nil {
			msg = fmt.Sprintf("skipping source path %v not found error for command %v, error: %v", src, command, err)
			return writeLog(log.WARN, log.FILE, msg)
		}
	}

	_ = writeLog(log.INFO, "", "get cache")
	if compress {
		fmt.Println("zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)
		targetZipPath := fmt.Sprintf("%s.zip", dest)
		_ = os.MkdirAll(filepath.Dir(dest), 0777)
		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return writeLog(log.ERROR, log.COPY, err)
		}
		_, err = Unzip(targetZipPath, filepath.Dir(dest))
		if err != nil {
			_ = writeLog(log.WARN, log.ZIP, fmt.Sprintf("Could not unzip file %s: %s", filepath.Dir(src), err))
		}
		defer os.RemoveAll(targetZipPath)
	} else {
		_ = writeLog(log.INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return writeLog(log.ERROR, log.COPY, err)
		}
	}
	return writeLog(log.INFO, "", "get cache complete")
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
	var msg string
	var err error
	var b int
	var md5Json []byte
	var md5Status bool

	_ = writeLog(log.INFO, "", "set cache")
	if _, err = os.Stat(src); err != nil {
		msg = fmt.Sprintf("%v, source path not found for command %v", err, command)
		return writeLog(log.ERROR, log.FILE, msg)
	}

	if cacheMaxSizeInMB > 0 {
		sizeInMB := int64(float64(getDirSizeInBytes(src)) * 0.000001)
		if sizeInMB > cacheMaxSizeInMB {
			msg = fmt.Sprintf("source directory size %vMB is more than allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB)
			return writeLog(log.ERROR, log.MAXSIZELIMIT, msg)
		}
		_ = writeLog(log.INFO, "", fmt.Sprintf("source directory size %v, allowed max limit %v", sizeInMB, cacheMaxSizeInMB))
	}

	_ = writeLog(log.INFO, "", fmt.Sprintf("md5Check %v", md5Check))
	if md5Check {
		_ = writeLog(log.INFO, "", "starting md5Check")
		md5Json, md5Status = checkMd5(src, dest)
		if md5Status {
			return writeLog(log.WARN, log.FILE, fmt.Sprintf("source %s and destination %s directories are same, aborting", src, dest))
		}
		_ = writeLog(log.INFO, "", "md5Check complete")
	}
	removeCacheDirectory(dest, command)

	if compress {
		_ = writeLog(log.INFO, "", "zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)
		targetZipPath := fmt.Sprintf("%s.zip", dest)

		err = Zip(src, srcZipPath)
		if err != nil {
			msg = fmt.Sprintf("%v, failed to zip files from %v to %v", err, src, srcZipPath)
			return writeLog(log.ERROR, log.ZIP, msg)
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0777)
		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return writeLog(log.ERROR, log.COPY, err)
		}
		defer os.RemoveAll(srcZipPath)
	} else {
		_ = writeLog(log.INFO, "", "zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return writeLog(log.ERROR, log.COPY, err)
		}
	}
	_ = writeLog(log.INFO, "", "set cache complete")
	if md5Check {
		md5Path := filepath.Join(filepath.Dir(dest), "md5.json")
		jsonFile, err := os.Create(md5Path)
		if err != nil {
			_ = writeLog(log.WARN, log.MD5, fmt.Sprintf("error: %v, not able to create %v md5.json file", err, dest))
		}
		defer jsonFile.Close()
		if b, err = jsonFile.Write(md5Json); err != nil {
			_ = writeLog(log.WARN, log.MD5, fmt.Sprintf("error %v writing md5.json file to destination %v", err, dest))
		} else {
			_ = jsonFile.Sync()
			_ = writeLog(log.INFO, "", fmt.Sprintf("wrote %d bytes of md5.json file to destination %v", b, dest))
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
		return writeLog(log.ERROR, log.COMMAND, msg)
	}

	if cacheScope == "" {
		msg = fmt.Sprintf("%v, cache scope %v empty", err, cacheScope)
		return writeLog(log.ERROR, log.SCOPE, msg)
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
		return writeLog(log.ERROR, log.FILE, msg)
	}

	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v in path %v, command: %v", err, baseCacheDir, command)
		return writeLog(log.ERROR, log.FILE, msg)
	}

	if _, err := os.Stat(baseCacheDir); err != nil {
		msg = fmt.Sprintf("%v, cache path %s not found", err, baseCacheDir)
		return writeLog(log.ERROR, log.FILE, msg)
	}

	cacheDir := filepath.Join(baseCacheDir, srcDir)
	src := srcDir
	dest := cacheDir

	switch command {
	case "set":
		if err = setCache(src, dest, command, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return writeLog(log.ERROR, "", fmt.Sprintf("error %v in set cache", err))
		}
	case "get":
		src = cacheDir
		dest = srcDir
		if err = getCache(src, dest, command, compress); err != nil {
			_ = writeLog(log.WARN, "", fmt.Sprintf("error %v in get cache", err))
		}
	case "remove":
		removeCacheDirectory(dest, command)
	}
	return nil
}
