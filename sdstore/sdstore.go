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

	"github.com/hashicorp/go-retryablehttp"
)

var UTCLoc, _ = time.LoadLocation("UTC")

// SDStore is able to upload, download, and remove the contents of a Reader to the SD Store
type SDStore interface {
	Upload(u *url.URL, filePath string, toCompress bool) error
	Download(url *url.URL, toExtract bool) error
	Remove(url *url.URL) error
}

type sdStore struct {
	token  string
	client *retryablehttp.Client
}

// NewStore returns an SDStore instance.
func NewStore(token string, maxRetries int, httpTimeout int, retryWaitMin int, retryWaitMax int) SDStore {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.HTTPClient.Timeout = time.Duration(httpTimeout) * time.Second
	retryClient.CheckRetry = retryablehttp.DefaultRetryPolicy

	return &sdStore{
		token:  token,
		client: retryClient,
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
	err := s.remove(u.String())
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

	body, err := s.get(urlString)
	if err != nil {
		return err
	}

	// Read file
	filePath := getFilePath(url)
	log.Printf("filePath = %s", filePath)
	if filePath != "" {
		dir, _ := filepath.Split(filePath)
		if !strings.HasPrefix(filePath, "/") {
			wd, _ := os.Getwd()
			dir = filepath.Join(wd, dir)
		}
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

		_, err = file.Write(body)
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
			log.Printf("failed to upload files %v to store (upload size = %s)", filePath, fileSize(filePath))
			return err
		}
		log.Printf("Upload to %s successful (upload size = %s).", u.String(), fileSize(filePath))
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
		log.Printf("failed to upload file %s to store (upload size = %s)", zipPath, fileSize(zipPath))
		return err
	}
	log.Printf("Upload to %s successful (upload size = %s).", u.String(), fileSize(zipPath))

	return nil
}

// return file size suitable for logging (ignores errors)
func fileSize(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "n/a"
	}
	fi, err := file.Stat()
	if err != nil {
		return "n/a"
	}

	return fmt.Sprintf("%d", fi.Size())
}

// token header for request
func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// DELETE request
func (s *sdStore) remove(url string) error {
	_, err := s.request(url, "DELETE")
	return err
}

// GET request; caller should close response.Body
func (s *sdStore) get(url string) ([]byte, error) {
	return s.request(url, "GET")
}

func (s *sdStore) request(url string, requestType string) ([]byte, error) {
	req, err := http.NewRequest(requestType, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Generating request to Screwdriver: %v", err)
	}

	defer s.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(s.token))

	res, err := s.client.StandardClient().Do(req)

	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url, err)
		return nil, fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url, err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Store API: %v", err)
		return nil, fmt.Errorf("reading response Body from Store API: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparsable error response from Store API: %v", parseError)
			return nil, fmt.Errorf("unparsable error response from Store API: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url)
		return nil, fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url)
	}

	return body, nil
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data from disk to save memory
func (s *sdStore) putFile(url *url.URL, bodyType string, filePath string) error {
	requestType := "PUT"
	req, err := retryablehttp.NewRequest(requestType, url.String(), func() (io.Reader, error) {
		return os.Open(filePath)
	})
	if err != nil {
		log.Printf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
		return fmt.Errorf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
	}

	defer s.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	if fi, err := os.Stat(filePath); err == nil {
		req.ContentLength = fi.Size()
	}

	res, err := s.client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
		return fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Store API: %v", err)
		return fmt.Errorf("reading response Body from Store API: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparsable error response from Store API: %v", parseError)
			return fmt.Errorf("unparsable error response from Store API: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url.String())
		return fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url.String())
	}

	return nil
}
