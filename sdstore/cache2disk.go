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
param -	srcPath     	source directory
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, srcPath string) error {
	var err error

	homeDir, _ := os.UserHomeDir()
	cacheDir := ""
	command = strings.ToLower(command)

	if command != "set" && command != "get" && command !="remove" {
		return fmt.Errorf("error: %v, command: %v is not expected", err, command)
	}

	switch strings.ToLower(cacheScope) {
	case "pipeline":
		cacheDir = os.Getenv("SD_PIPELINE_CACHE_DIR")
	case "event":
		cacheDir = os.Getenv("SD_EVENT_CACHE_DIR")
	case "job":
		cacheDir = os.Getenv("SD_JOB_CACHE_DIR")
	}

	if cacheDir == "" {
		return fmt.Errorf("error: %v, cache directory empty for cache scope %v", err, cacheScope)
	}

	if strings.HasPrefix(cacheDir, "~/") {
		cacheDir = filepath.Join(homeDir, strings.TrimPrefix(cacheDir, "~/"))
	}

	if strings.HasPrefix(srcPath, "~/") {
		srcPath = filepath.Join(homeDir, strings.TrimPrefix(srcPath, "~/"))
	}

	if srcPath, err = filepath.Abs(srcPath); err != nil {
		return fmt.Errorf("error: %v in src path %v, command: %v", err, srcPath, command)
	}

	if cacheDir, err = filepath.Abs(cacheDir); err != nil {
		return fmt.Errorf("error: %v in path %v, command: %v", err, cacheDir, command)
	}

	cachePath := filepath.Join(cacheDir, srcPath)
	src := srcPath
	dest := cachePath

	if command == "get" {
		src = cachePath
		dest = srcPath
	}

	if _, err = os.Stat(src); err != nil {
		return fmt.Errorf("error: %v, source path not found", err)
	}

	if command != "get" {
		if err = os.RemoveAll(dest); err != nil {
			return fmt.Errorf("error: %v, failed to clean out the destination directory: %v", err, dest)
		}
		if command == "remove" {
			fmt.Printf("command: %v, cache directories %v removed \n", command, dest)
			return nil
		}
	}

	if err = copy.Copy(src, dest); err != nil {
		return err
	}

	fmt.Println ("Cache complete ...")
	return nil
}
