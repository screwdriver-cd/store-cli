package sdstore

import (
	"fmt"
	"path/filepath"
	"os"
	"strings"
	"io"
	"io/ioutil"
)

/*
Copy file from source to destination
param - fi	        file descriptors / info
param - src             source file
param - dest            destination file
return - nil / error    success - return nil; error - return error description
*/
func copyFile(fi os.FileInfo, src, dest string) error {
	var err error
	var srcFile *os.File
	var destFile *os.File

	if srcFile, err = os.Open(src); err != nil {
		return err
	}
	defer srcFile.Close()

	if destFile, err = os.Create(dest); err != nil {
		return err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		return err
	}
	return nil
}

/*
Copy directory from source to destination. Create directories in destination if not available
param - src             source directory
param - dest            destination directory
return  - nil / error   success - return nil; error - return error description
*/
func copyDir(src string, dest string) error {
	var err error
	var di []os.FileInfo
	var fi os.FileInfo

	if fi, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dest, fi.Mode()); err != nil {
		return err
	}

	if di, err = ioutil.ReadDir(src); err != nil {
		return err
	}

	for _, fd := range di {
		srcPath := filepath.Join(src, fd.Name())
		destPath := filepath.Join(dest, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err = copyFile(fi, srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

/*
cache directories and files
param - command         set, get or remove
param - cacheScope     	pipeline, event, job
param -	srcPath     	source directory
return - nil / error   success - return nil; error - return error description
*/
func Cache2Disk(command, cacheScope, srcPath string) error {
	var err error
	var fi os.FileInfo

	homeDir, _ := os.UserHomeDir()
	cacheDir := ""
	command = strings.ToLower(command)

	if command != "set" && command != "get" && command !="remove" {
		return fmt.Errorf("Error: %v, command: %v is not expected", err, command)
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
		return fmt.Errorf("Error: %v, cache directory empty for cache scope %v ", err, cacheScope)
	}

	if strings.HasPrefix(cacheDir, "~/") {
		cacheDir = filepath.Join(homeDir, strings.TrimPrefix(cacheDir, "~/"))
	}

	if strings.HasPrefix(srcPath, "~/") {
		srcPath = filepath.Join(homeDir, strings.TrimPrefix(srcPath, "~/"))
	}

	if srcPath, err = filepath.Abs(srcPath); err != nil {
		return fmt.Errorf("Error: %v in path %v, command: %v", err, srcPath, command)
	}

	if cacheDir, err = filepath.Abs(cacheDir); err != nil {
		return fmt.Errorf("Error: %v in path %v, command: %v", err, cacheDir, command)
	}

	cachePath := filepath.Join(cacheDir, srcPath)
	src := srcPath
	dest := cachePath

	if command == "get" {
		src = cachePath
		dest = srcPath
	}

	if fi, err = os.Stat(src); err != nil {
		return fmt.Errorf("Error: %v in path %v for command: %v", err, src, command)
	}

	if command != "get" {
		if err = os.RemoveAll(dest); err != nil {
			return fmt.Errorf("Error: %v, failed to clean out the destination directory: %v ", err, dest)
		}

		if command == "remove" {
			fmt.Printf("command: %v, cache directories %v removed \n", command, dest)
			return nil
		}
	}

	if fi.IsDir() {
		if err = copyDir(src, dest); err != nil {
			return err
		} else {
			if err = copyFile(fi, src, dest); err != nil {
				return err
			}
		}
	} else {
		if err = copyFile(fi, src, dest); err != nil {
			return err
		}
	}

	return nil
}
