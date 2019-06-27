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
	Download(url *url.URL, toExtract bool) error
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
	err := s.remove(u)
	if err != nil {
		return err
	}
	log.Printf("Deletion from %s successful.", u.String())
	return nil
}

// Download a file from a path within the SD Store
// Note: it's possible that this won't actually download a file and still return error == nil
func (s *sdStore) Download(url *url.URL, toExtract bool) error {
	urlString := url.String()
	if toExtract {
		urlString += ".zip"
	}

	res, err := s.get(urlString)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	// Read file
	filePath := getFilePath(url)
	log.Printf("filePath = %s", filePath)
	if filePath != "" {
		dir, _ := filepath.Split(filePath)
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}

		if toExtract {
			filePath += ".zip"
		}
		file, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(file, res.Body)
		if err != nil {
			return err
		}

		// ensure file is flushed
		err = file.Sync()
		if err != nil {
			return err
		}

		if toExtract {
			_, err = Unzip(filePath, dir)
			if err != nil {
				log.Printf("Could not unzip file %s: %s", filePath, err)
			} else {
				os.Remove(filePath)
			}
		}

		log.Printf("Download from %s to %s successful.", url.String(), filePath)
	} else {
		log.Printf("Request for %s successful, but not written to file.", url.String())
	}

	return nil
}

func (s *sdStore) GenerateAndCheckMd5Json(url *url.URL, path string) (string, error) {
	newMd5, err := MD5All(path)
	if err != nil {
		return "", err
	}

	err = s.Download(url, false)
	if err == nil {
		oldMd5FilePath := fmt.Sprintf("%s_md5.json", filepath.Clean(path))
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

	md5Path := fmt.Sprintf("%s_md5.json", filepath.Base(path))
	jsonFile, err := os.Create(md5Path)
	if err != nil {
		return "", err
	}
	defer jsonFile.Close()

	jsonFile.Write(jsonString)

	return md5Path, nil
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build/event path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdStore) Upload(u *url.URL, filePath string, toCompress bool) error {
	if !toCompress {
		err := s.putFile(u, "text/plain", filePath)
		if err != nil {
			log.Printf("failed to upload files %v", filePath)
			return err
		}
		log.Printf("Upload to %s successful.", u.String())
		return nil
	}

	fileName := filepath.Base(filePath)
	encodedURL, err := url.Parse(fmt.Sprintf("%s%s", u.String(), "_md5.json"))
	if err != nil {
		return err
	}
	md5Json, err := s.GenerateAndCheckMd5Json(encodedURL, filePath)
	if err != nil && err.Error() == "Contents unchanged" {
		log.Printf("No change to %s, aborting upload", filePath)
		return nil
	}
	if err != nil {
		log.Printf("failed to generating md5 at %s", filePath)
		return err
	}

	err = s.putFile(encodedURL, "application/json", md5Json)
	if err != nil {
		log.Printf("failed to upload md5 json %s", md5Json)
		return err
	}

	err = os.Remove(md5Json)
	if err != nil {
		log.Printf("Unable to remove md5 file from path: %s, continuing", md5Json)
	}

	zipPath, err := filepath.Abs(fmt.Sprintf("%s.zip", fileName))
	if err != nil {
		return err
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	err = Zip(absPath, zipPath)
	if err != nil {
		log.Printf("failed to zip files from %v to %v", absPath, zipPath)
		return err
	}
	defer func() {
		if err := os.Remove(zipPath); err != nil {
			log.Printf("Unable to remove zip file: %v", err)
		}
	}()

	encodedURL, err = url.Parse(fmt.Sprintf("%s%s", u.String(), ".zip"))
	if err != nil {
		return err
	}
	err = s.putFile(encodedURL, "text/plain", zipPath)
	if err != nil {
		log.Printf("failed to upload file")
		return err
	}
	log.Printf("Upload to %s successful.", u.String())

	return nil
}

// token header for request
func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// Check the response for HTTP error.
func checkError(res *http.Response) error {
	if res.StatusCode/100 != 2 {
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("HTTP %d and failed to read body: %v", res.StatusCode, err)
		}
		return fmt.Errorf("HTTP %d returned: %s", res.StatusCode, body)
	}

	// 2xx is success
	return nil
}

// DELETE request
func (s *sdStore) remove(url *url.URL) error {
	res, err := s.retryingRequest(url.String(), "DELETE")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// GET request; caller should close response.Body
func (s *sdStore) get(url string) (*http.Response, error) {
	return s.retryingRequest(url, "GET")
}

// Common between GET and DELETE
func (s *sdStore) retryingRequest(url string, method string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	res, err := s.do(req)
	if err != nil {
		return nil, err
	}

	// Check for error
	err = checkError(res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data from disk to save memory
// Need to retry here so that we can re-open the file cuz httpClient closes the file after each request
func (s *sdStore) putFile(url *url.URL, bodyType string, filePath string) error {
	attemptNum := 0

	for {
		attemptNum = attemptNum + 1

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

		res, err := s.put(url, bodyType, input, fsize)
		defer res.Body.Close()

		retry, err := s.checkForRetry(res, err)

		if !retry {
			if err != nil {
				return err
			}

			return nil
		}
		log.Printf("(Try %d of %d) error received from %s %v: %v", attemptNum, s.maxRetries, "PUT", url, err)

		if attemptNum == s.maxRetries {
			return err
		}
		time.Sleep(s.backOff(attemptNum))
	}
}

// PUT request to SD store
func (s *sdStore) put(url *url.URL, bodyType string, payload io.Reader, size int64) (*http.Response, error) {
	req, err := http.NewRequest("PUT", url.String(), payload)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := s.client.Do(req)

	return res, err
}

func (s *sdStore) backOff(attemptNum int) time.Duration {
	return time.Duration(float64(attemptNum*attemptNum)*s.retryScaler) * time.Second
}

func (s *sdStore) checkForRetry(res *http.Response, err error) (bool, error) {
	if err != nil {
		log.Printf("failed to request to store: %v", err)
		return true, err
	}
	if res.StatusCode/100 == 4 && res.StatusCode != http.StatusRequestTimeout && res.StatusCode != http.StatusTooManyRequests {
		if res.Request != nil && res.Request.URL != nil {
			return false, fmt.Errorf("got %s from %s. stop retrying", res.Status, res.Request.URL)
		}
		return false, fmt.Errorf("got %s. stop retrying", res.Status)
	}

	if res.StatusCode/100 != 2 {
		return true, fmt.Errorf("got %s", res.Status)
	}

	return false, nil
}

func (s *sdStore) do(req *http.Request) (*http.Response, error) {

	attemptNum := 0
	for {
		attemptNum = attemptNum + 1

		res, err := s.client.Do(req)

		retry, err := s.checkForRetry(res, err)
		if !retry {
			return res, err
		}
		log.Printf("(Try %d of %d) error received from %s %v: %v", attemptNum, s.maxRetries, req.Method, req.URL, err)

		if attemptNum == s.maxRetries {
			return nil, fmt.Errorf("getting error from %s after %d retries: %v", req.URL, s.maxRetries, err)
		}
		time.Sleep(s.backOff(attemptNum))
	}
}
