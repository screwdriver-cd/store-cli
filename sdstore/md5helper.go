// Taken and modified from https://blog.golang.org/pipelines

package sdstore

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/karrick/godirwalk"
	"github.com/pieterclaerhout/go-waitgroup"
	"github.com/screwdriver-cd/store-cli/logger"
	"io"
	"strconv"
	// "math"
	"os"
	"path/filepath"
	"sync"
)

// A result is the product of reading and summing a file using MD5.
type result struct {
	path string
	sum  string
	err  error
}

const Md5helperModule = "md5helper"

// getEnv get key environment variable if exist otherwise return defaultValue
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

// get md5Hash for given file
func getMd5Hash(filePath string) (string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	md5hash := md5.New()
	b, err := io.Copy(md5hash, file)
	file.Close()
	if err != nil {
		return "", 0, err
	}

	md5hashInBytes := md5hash.Sum(nil)[:16]
	md5str := hex.EncodeToString(md5hashInBytes)
	return md5str, b, err
}

/*
Get all files for given path
param - path			file or folder path
return - []string / error	success - return array of filepath; error - return error description
*/
func getAllFiles(path string) ([]string, error) {
	var s []string
	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(filePath string, de *godirwalk.Dirent) error {
			if !de.ModeType().IsDir() {
				s = append(s, filePath)
			}
			return nil
		},
		ErrorCallback: func(filePath string, err error) godirwalk.ErrorAction {
			return godirwalk.SkipNode
		},
		Unsorted:            true,
		AllowNonDirectory:   true,
		FollowSymbolicLinks: true,
	})

	if err != nil {
		return nil, err
	}
	return s, err
}

func hashFromPath(filePath string) (string, error) {
	var md5str string

	file, err := os.Open(filePath)
	if err != nil {
		return md5str, err
	}
	md5hash := md5.New()
	_, err = io.Copy(md5hash, file)
	file.Close()
	if err != nil {
		return md5str, err
	}

	md5hashInBytes := md5hash.Sum(nil)[:16]
	md5str = hex.EncodeToString(md5hashInBytes)

	return md5str, nil
}

// sumFiles starts goroutines to walk the directory tree at root and digest each
// regular file.  These goroutines send the results of the digests on the result
// channel and send the result of the walk on the error channel.  If done is
// closed, sumFiles abandons its work.
func sumFiles(done <-chan struct{}, root string) (<-chan result, <-chan error) {
	// For each regular file, start a goroutine that sums the file and sends
	// the result on c.  Send the result of the walk on errc.
	c := make(chan result)
	errc := make(chan error, 1)
	go func() {
		var wg sync.WaitGroup
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			wg.Add(1)
			go func() {
				hash, err := hashFromPath(path)
				select {
				case c <- result{path, hash, err}:
				case <-done:
				}
				wg.Done()
			}()
			// Abort the walk if done is closed.
			select {
			case <-done:
				return errors.New("walk canceled")
			default:
				return nil
			}
		})
		// Walk has returned, so all calls to wg.Add are done.  Start a
		// goroutine to close c once all the sends are done.
		go func() {
			wg.Wait()
			close(c)
		}()
		// No select needed here, since errc is buffered.
		errc <- err
	}()
	return c, errc
}

// MD5All reads all the files in the file tree rooted at root and returns a map
// from file path to the MD5 sum of the file's contents.  If the directory walk
// fails or any read operation fails, MD5All returns an error.  In that case,
// MD5All does not wait for inflight read operations to complete.
func MD5All(root string) (map[string]string, error) {
	// MD5All closes the done channel when it returns; it may do so before
	// receiving all the values from c and errc.
	done := make(chan struct{})
	defer close(done)

	c, errc := sumFiles(done, root)

	m := make(map[string]string)
	for r := range c {
		if r.err != nil {
			return nil, r.err
		}
		m[r.path] = r.sum
	}
	if err := <-errc; err != nil {
		return nil, err
	}
	return m, nil
}

/*
GenerateMd5 reads files for given path, generates Md5 and returns ms5map or error
param - path		file or folder path
return - md5map / error	success - return md5map; error - return error description
*/

func GenerateMd5(path string) (map[string]string, error) {
	var rwm sync.RWMutex
	md5Map := make(map[string]string)
	maxGoThreads, _ := strconv.Atoi(getEnv("SD_CACHE_MAX_GO_THREADS", "10000"))
	wg := waitgroup.NewWaitGroup(maxGoThreads)

	files, err := getAllFiles(path)
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("total files: %d\n", len(files))
	logger.Log(logger.LOGLEVEL_INFO, Md5helperModule, "", msg)
	for _, name := range files {
		wg.BlockAdd()
		go func(f string) {
			md5str, b, err := getMd5Hash(f)
			rwm.Lock()
			md5Map[f] = fmt.Sprintf("%v,%d,%v", md5str, b, err)
			rwm.Unlock()
			wg.Done()
		}(name)
	}
	wg.Wait()
	return md5Map, err
}
