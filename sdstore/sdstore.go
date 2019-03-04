package sdstore

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// SDStore is able to upload, download, and remove the contents of a Reader to the SD Store
type SDStore interface {
	Upload(u *url.URL, filePath string, toCompress bool) error
	Download(url *url.URL, toExtract bool) ([]byte, error)
	Remove(url *url.URL) error
}

type sdStore struct {
	token       string
	client      *http.Client
	retryScaler float64
	maxRetries  int
}

// NewStore returns an SDStore instance.
func NewStore(token string) SDStore {
	return &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 300 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3,
	}
}

// SDError is an error response from the Screwdriver API
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

func getFilePath(u *url.URL) string {
	path := u.Path
	r, err := regexp.Compile("^/v[0-9]+/caches/(?:events|pipelines|jobs)/(?:[0-9]+)/(.+)$")

	if err != nil {
		return ""
	}

	matched := r.FindStringSubmatch(path)
	if len(matched) < 2 {
		return ""
	}

	filepath := matched[1]
	// trim trailing slashes
	filepath = strings.TrimRight(filepath, "/")
	// decode
	filepath, _ = url.QueryUnescape(filepath)

	return filepath
}

// Error implements the error interface for SDError
func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

// Remove a file from a path within the SD Store
func (s *sdStore) Remove(u *url.URL) error {
	_, err := s.remove(u)
	if err != nil {
		return err
	}
	log.Printf("Deletion from %s successful.", u.String())
	return nil
}

// Download a file from a path within the SD Store
func (s *sdStore) Download(url *url.URL, toExtract bool) ([]byte, error) {
	res, err := s.get(url, toExtract)
	if err != nil {
		return nil, err
	}
	log.Printf("Download from %s successful.", url.String())

	return res, nil
}

func (s *sdStore) GenerateAndCheckMd5Json(url *url.URL, path string) (string, error) {
	newMd5, err := MD5All(path)
	if err != nil {
		return "", err
	}

	_, err = s.Download(url, false)
	if err == nil {
		var oldMd5FilePath string
		oldMd5FilePath = fmt.Sprintf("%s_md5.json", filepath.Clean(path))
		oldMd5File, err := ioutil.ReadFile(oldMd5FilePath)
		if err != nil {
			return "", err
		}

		oldMd5 := make(map[string]string)
		err = json.Unmarshal(oldMd5File, &oldMd5)
		os.RemoveAll(oldMd5FilePath)
		if err != nil {
			return "", err
		}

		if reflect.DeepEqual(oldMd5, newMd5) {
			return "", fmt.Errorf("Contents unchanged")
		}
	}

	jsonString, err := json.Marshal(newMd5)

	if err != nil {
		return "", err
	}

	var md5Path string
	md5Path = fmt.Sprintf("%s_md5.json", filepath.Base(path))
	jsonFile, err := os.Create(md5Path)

	if err != nil {
		return "", err
	}
	defer jsonFile.Close()

	jsonFile.Write(jsonString)
	jsonFile.Close()

	return md5Path, nil
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build/event path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdStore) Upload(u *url.URL, filePath string, toCompress bool) error {
	var err error

	for i := 0; i < s.maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*s.retryScaler) * time.Second)

		if toCompress {
			var fileName string
			fileName = filepath.Base(filePath)
			encodedURL, _ := url.Parse(fmt.Sprintf("%s%s", u.String(), "_md5.json"))
			md5Json, err := s.GenerateAndCheckMd5Json(encodedURL, filePath)

			if err != nil && err.Error() == "Contents unchanged" {
				log.Printf("No change to %s, aborting upload", filePath)
				return nil
			}

			if err != nil {
				log.Printf("(Try %d of %d) error received from generating md5: %v", i+1, s.maxRetries, err)
				continue
			}

			err = s.putFile(encodedURL, "application/json", md5Json)
			if err != nil {
				log.Printf("(Try %d of %d) error received from uploading md5 json: %v", i+1, s.maxRetries, err)
				continue
			}

			err = os.Remove(md5Json)
			if err != nil {
				log.Printf("Unable to remove md5 file from path: %s, continuing", md5Json)
			}

			var zipPath string
			zipPath, err = filepath.Abs(fmt.Sprintf("%s.zip", fileName))

			if err != nil {
				log.Printf("(Try %d of %d) Unable to determine filepath: %v", i+1, s.maxRetries, err)
				continue
			}

			absPath, _ := filepath.Abs(filePath)
			err = Zip(absPath, zipPath)
			if err != nil {
				log.Printf("(Try %d of %d) Unable to zip file: %v", i+1, s.maxRetries, err)
				continue
			}

			encodedURL, _ = url.Parse(fmt.Sprintf("%s%s", u.String(), ".zip"))
			err = s.putFile(encodedURL, "text/plain", zipPath)
			errRemove := os.Remove(zipPath)

			if err != nil {
				log.Printf("(Try %d of %d) error received from file upload: %v", i+1, s.maxRetries, err)
				continue
			}

			if errRemove != nil {
				log.Printf("Unable to remove zip file: %v", err)
			}

			log.Printf("Upload to %s successful.", u.String())

			return nil
		} else {
			err := s.putFile(u, "text/plain", filePath)
			if err != nil {
				log.Printf("(Try %d of %d) error received from file upload: %v", i+1, s.maxRetries, err)
				continue
			}
			log.Printf("Upload to %s successful.", u.String())
			return nil
		}
	}
	return fmt.Errorf("posting to %s after %d retries: %v", filePath, s.maxRetries, err)
}

// token header for request
func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// handleResponse attempts to parse error objects from Screwdriver
func handleResponse(res *http.Response) ([]byte, error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d returned: %s", res.StatusCode, body)
	}
	return body, nil
}

// DELETE request
func (s *sdStore) remove(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("DELETE", url.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))

	res, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return handleResponse(res)
}

// GET request from SD Store
func (s *sdStore) get(url *url.URL, toExtract bool) ([]byte, error) {
	filePath := getFilePath(url)
	var file *os.File
	var err error
	var dir string
	var urlString string

	if toExtract == true {
		urlString = fmt.Sprintf("%s%s", url.String(), ".zip")
	} else {
		urlString = url.String()
	}

	req, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	res, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := handleResponse(res)
	if err != nil {
		return nil, err
	}

	// Write to file
	if filePath != "" {
		dir, _ = filepath.Split(filePath)
		err := os.MkdirAll(dir, 0777)

		if toExtract == true {
			filePath += ".zip"
		}

		file, err = os.Create(filePath)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		_, err = file.Write(body)
		if err != nil {
			return nil, err
		}

		if toExtract {
			_, err = Unzip(filePath, dir)
			if err != nil {
				log.Printf("Could not unzip file %s: %s", filePath, err)
			} else {
				os.Remove(filePath)
			}
		}
	}

	return body, nil
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data from disk to save memory
func (s *sdStore) putFile(url *url.URL, bodyType string, filePath string) error {
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()

	stat, err := input.Stat()
	if err != nil {
		return err
	}
	fsize := stat.Size()

	reader, writer := io.Pipe()

	done := make(chan error)
	go func() {
		_, err := s.put(url, bodyType, reader, fsize)
		if err != nil {
			done <- err
			return
		}

		done <- nil
	}()

	io.Copy(writer, input)
	if err := writer.Close(); err != nil {
		return err
	}

	return <-done
}

// PUT request to SD store
func (s *sdStore) put(url *url.URL, bodyType string, payload io.Reader, size int64) ([]byte, error) {
	req, err := http.NewRequest("PUT", url.String(), payload)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return handleResponse(res)
}

func (s *sdStore) backOff(attemptNum int) time.Duration {
	return time.Duration(float64(attemptNum*attemptNum)*s.retryScaler) * time.Second
}

func (s *sdStore) checkForRetry(res *http.Response, err error) (bool, error) {
	if err != nil {
		log.Printf("failed to request to store: %v", err)
		return true, err
	}
	if res.StatusCode == http.StatusNotFound {
		if res.Request != nil && res.Request.URL != nil {
			return false, fmt.Errorf("got %s from %s. stop retrying", res.Status, res.Request.URL)
		}
		return false, fmt.Errorf("got %s. stop retrying", res.Status)
	}

	if res.StatusCode/100 != 2 {
		return true, nil
	}

	return false, nil
}

func (s *sdStore) do(req *http.Request) (*http.Response, error) {
	attemptNum := 0
	for {
		attemptNum = attemptNum + 1
		res, err := s.client.Do(req)
		retry, err := s.checkForRetry(res, err)
		log.Printf("(Try %d of %d) error received from %s %v: %v", attemptNum, s.maxRetries, req.Method, req.URL, err)
		if !retry {
			return res, err
		}

		if attemptNum == s.maxRetries {
			break
		}
		time.Sleep(s.backOff(attemptNum))
	}
	return nil, fmt.Errorf("getting from %s after %d retries", req.URL, s.maxRetries)
}
