package sdstore

import (
	"fmt"
	"github.com/otiai10/copy"
	"os"
	"path/filepath"
	"strings"
)

/*
cache directories and files to/from shared storage
param - command         set, get or remove
param - cacheScope     	pipeline, event, job
param -	srcDir     	source directory
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, srcDir string) error {
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
		} else {
			fmt.Printf("skipping source path not found error for command %v, error: %v", command, err)
			return nil
		}
	}

	if command != "get" {
		if err = os.RemoveAll(dest); err != nil {
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

	fmt.Println("Cache complete ...")
	return nil
}
