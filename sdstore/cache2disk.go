package sdstore

import (
	"encoding/json"
	"fmt"
	"github.com/otiai10/copy"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

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
	var oldMd5 map[string]string
	var newMd5 map[string]string

	fmt.Println("start md5 check")
	oldMd5FilePath := filepath.Join(filepath.Dir(dest), "md5.json")
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		log.Printf("error: %v, not able to get md5.json from: %v \n", err, filepath.Dir(dest))
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = MD5All(src); err != nil {
		log.Printf("error: %v, not able to generate md5 for directory: %v \n", err, src)
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
		log.Printf("error: %v, failed to clean out the destination directory: %v \n", err, path)
	}
	fmt.Printf("command: %v, cache directories %v removed \n", command, path)
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
	var err error

	if compress {
		if _, err = os.Stat(filepath.Dir(src)); err != nil {
			log.Printf("skipping source path %v not found error for command %v, error: %v \n", src, command, err)
			return nil
		}
	} else {
		if _, err = os.Stat(src); err != nil {
			log.Printf("skipping source path %v not found error for command %v, error: %v \n", src, command, err)
			return nil
		}
	}

	fmt.Println("get cache")
	if compress {
		fmt.Println("zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)
		targetZipPath := fmt.Sprintf("%s.zip", dest)
		_ = os.MkdirAll(filepath.Dir(dest), 0777)
		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return err
		}
		_, err = Unzip(targetZipPath, filepath.Dir(dest))
		if err != nil {
			log.Printf("Could not unzip file %s: %s", filepath.Dir(src), err)
		}
		defer os.RemoveAll(targetZipPath)
	} else {
		fmt.Println("zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return err
		}
	}
	fmt.Println("get cache complete")
	return nil
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
	var err error
	var b int
	var md5Json []byte
	var md5Status bool

	fmt.Println("set cache")
	if _, err = os.Stat(src); err != nil {
		return fmt.Errorf("error: %v, source path not found for command %v \n", err, command)
	}

	if cacheMaxSizeInMB > 0 {
		sizeInMB := int64(float64(getDirSizeInBytes(src)) * 0.000001)
		if sizeInMB > cacheMaxSizeInMB {
			return fmt.Errorf("error, source directory size %vMB is more than allowed max limit %vMB", sizeInMB, cacheMaxSizeInMB)
		}
		fmt.Printf("source directory size %v, allowed max limit %v\n", sizeInMB, cacheMaxSizeInMB)
	}

	fmt.Printf("md5Check %v\n", md5Check)
	if md5Check {
		fmt.Println("starting md5Check")
		md5Json, md5Status = checkMd5(src, dest)
		if md5Status {
			log.Printf("source %s and destination %s directories are same, aborting \n", src, dest)
			return nil
		}
		fmt.Println("md5Check complete")
	}
	removeCacheDirectory(dest, command)

	if compress {
		fmt.Println("zip enabled")
		srcZipPath := fmt.Sprintf("%s.zip", src)
		targetZipPath := fmt.Sprintf("%s.zip", dest)

		err = Zip(src, srcZipPath)
		if err != nil {
			fmt.Printf("failed to zip files from %v to %v \n", src, srcZipPath)
			return fmt.Errorf("error %v\n", err)
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0777)
		if err = copy.Copy(srcZipPath, targetZipPath); err != nil {
			return err
		}
		defer os.RemoveAll(srcZipPath)
	} else {
		fmt.Println("zip disabled")
		if err = copy.Copy(src, dest); err != nil {
			return err
		}
	}
	fmt.Println("set cache complete")
	if md5Check {
		md5Path := filepath.Join(filepath.Dir(dest), "md5.json")
		jsonFile, err := os.Create(md5Path)
		if err != nil {
			log.Printf("error: %v, not able to create %v md5.json file", err, dest)
		}
		defer jsonFile.Close()
		if b, err = jsonFile.Write(md5Json); err != nil {
			log.Printf("error %v writing md5.json file to destination %v \n", err, dest)
		} else {
			_ = jsonFile.Sync()
			fmt.Printf("wrote %d bytes of md5.json file to destination %v \n", b, dest)
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
	var err error

	homeDir, _ := os.UserHomeDir()
	baseCacheDir := ""
	command = strings.ToLower(strings.TrimSpace(command))
	cacheScope = strings.ToLower(strings.TrimSpace(cacheScope))

	if command != "set" && command != "get" && command != "remove" {
		return fmt.Errorf("error: %v, command: %v is not expected", err, command)
	}

	if cacheScope == "" {
		return fmt.Errorf("error: %v, cache scope %v empty", err, cacheScope)
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
		return fmt.Errorf("error: %v in src path %v, command: %v", err, srcDir, command)
	}

	if baseCacheDir, err = filepath.Abs(baseCacheDir); err != nil {
		return fmt.Errorf("error: %v in path %v, command: %v", err, baseCacheDir, command)
	}

	if _, err := os.Stat(baseCacheDir); err != nil {
		return fmt.Errorf("error: %v, cache path %v not found", err, baseCacheDir)
	}

	cacheDir := filepath.Join(baseCacheDir, srcDir)
	src := srcDir
	dest := cacheDir

	switch command {
	case "set":
		if err = setCache(src, dest, command, compress, md5Check, cacheMaxSizeInMB); err != nil {
			return err
		}
	case "get":
		src = cacheDir
		dest = srcDir
		if err = getCache(src, dest, command, compress); err != nil {
			log.Printf("error %v in get cache \n", err)
		}
	case "remove":
		removeCacheDirectory(dest, command)
	}
	return nil
}
