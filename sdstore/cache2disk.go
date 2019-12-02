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
)

/*
checkMd5 for source and dest
param - src		source directory
param - dest     	destination directory
return - md5, error   	success - return md5, "same"
			error - return md5, "change detected"
*/
func checkMd5(src, dest string) ([]byte, error) {
	var oldMd5 map[string]string
	var newMd5 map[string]string

	fmt.Println("start md5 check")
	oldMd5FilePath := filepath.Join(filepath.Dir(dest), "md5.json")
	oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
	if err != nil {
		oldMd5File = []byte("")
		fmt.Printf("error: %v, not able to get md5.json from: %v \n", err, filepath.Dir(dest))
	}
	_ = json.Unmarshal(oldMd5File, &oldMd5)

	if newMd5, err = MD5All(src); err != nil {
		fmt.Printf("error: %v, not able to generate md5 for directory: %v \n", err, src)
	}
	md5Json, _ := json.Marshal(newMd5)

	if reflect.DeepEqual(oldMd5, newMd5) {
		return md5Json, fmt.Errorf("same")
	} else {
		return md5Json, fmt.Errorf("change detected")
	}
}

/*
cache directories and files to/from shared storage
param - command         set, get or remove
param - cacheScope     	pipeline, event, job
param -	srcDir     	source directory
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, srcDir string) error {
	var err error
	var md5Json []byte
	var b int

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

	switch strings.ToLower(cacheScope) {
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

	if command == "get" {
		src = cacheDir
		dest = srcDir
	}

	if _, err = os.Stat(src); err != nil {
		if command == "set" {
			return fmt.Errorf("error: %v, source path not found for command %v", err, command)
		}

		if command == "get" {
			fmt.Printf("skipping source path not found error for command %v, error: %v", command, err)
			return nil
		}
	}

	if command != "get" {
		if command == "set" {
			fmt.Println("starting md5Check")
			md5Json, err = checkMd5(src, dest)
			if err != nil && err.Error() == "same" {
				fmt.Printf("source %v and destination %v directories are same, aborting \n", src, dest)
				return nil
			}
			fmt.Printf("md5 change detected %v between source %v and destination %v directories \n", string(md5Json), src, dest)
			fmt.Println("md5Check complete")
		}

		if err = os.RemoveAll(filepath.Dir(dest)); err != nil {
			fmt.Printf("error: %v, failed to clean out the destination directory: %v", err, dest)
		}

		if command == "remove" {
			fmt.Printf("command: %v, cache directories %v removed \n", command, dest)
			return nil
		}
	}

	if err = copy.Copy(src, dest); err != nil {
		return err
	}

	if command == "set" {
		md5Path := filepath.Join(filepath.Dir(dest), "md5.json")
		jsonFile, err := os.Create(md5Path)
		if err != nil {
			fmt.Printf("error: %v, not able to create %v md5.json file", err, md5Path)
		}
		defer jsonFile.Close()
		if b, err = jsonFile.Write(md5Json); err != nil {
			fmt.Printf("error %v writing md5.json file to destination %v \n", err, md5Path)
		} else {
			_ = jsonFile.Sync()
			fmt.Printf("wrote %d bytes of md5.json file to destination %v \n", b, md5Path)
		}
	}

	fmt.Println("Cache complete ...")
	return nil
}
