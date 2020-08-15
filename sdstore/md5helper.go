// Taken and modified from https://blog.golang.org/pipelines

package sdstore

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/karrick/godirwalk"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/screwdriver-cd/store-cli/logger"
)

// md5Hash struct with file, md5, total bytes, error
type md5Hash struct {
	file string
	sum  string
	b    int64
	err  error
}

// A result is the product of reading and summing a file using MD5.
type result struct {
	path string
	sum  string
	err  error
}

const Md5helperModule = "md5helper"

// get md5Hash for given file
func getMd5Hash(wg *sync.WaitGroup, filePath string) (string, int64, error) {
	var md5str string

	defer wg.Done()
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	md5hash := md5.New()
	b, err := io.Copy(md5hash, file)
	if err != nil {
		return "", b, err
	}

	md5hashInBytes := md5hash.Sum(nil)[:16]
	md5str = hex.EncodeToString(md5hashInBytes)
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
			if !de.ModeType().IsDir() && de.ModeType().IsRegular() {
				s = append(s, filePath)
			}
			return nil
		},
		ErrorCallback: func(filePath string, err error) godirwalk.ErrorAction {
			fmt.Printf("error %v in walking directory %s", err, filePath)
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

	defer file.Close()

	md5hash := md5.New()
	if _, err := io.Copy(md5hash, file); err != nil {
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
	var wg sync.WaitGroup

	const MaxConcurrencyLimit = 3000
	md5Map := make(map[string]string)
	md5Channel := make(chan md5Hash)
	defer close(md5Channel)

	files, err := getAllFiles(path)
	if err != nil {
		return nil, err
	}
	totalFiles := float64(len(files))
	batchSize := math.Ceil(totalFiles / float64(MaxConcurrencyLimit))
	msg := fmt.Sprintf("batch size: %d, concurreny limit: %d, total files: %d\n", int(batchSize), int(MaxConcurrencyLimit), int(totalFiles))
	logger.Log(logger.LOGLEVEL_INFO, Md5helperModule, "", msg)

	k := 0
	for i := 0; i < int(batchSize); i++ {
		for j := k; j < int(totalFiles); j++ {
			wg.Add(1)
			go func(f string) {
				md5str, b, err := getMd5Hash(&wg, f)
				md5Channel <- md5Hash{f, md5str, b, err}
			}(files[j])
			k = j + 1
			if math.Mod(float64(k), float64(MaxConcurrencyLimit)) == 0 {
				break
			}
		}
		wg.Wait()
	}

	for range files {
		md5 := <-md5Channel
		if md5.err != nil {
			md5Map = nil
			err = md5.err
			break
		}
		md5Map[md5.file] = fmt.Sprintf("%v,%d,%v", md5.sum, md5.b, md5.err)
	}

	return md5Map, err
}
