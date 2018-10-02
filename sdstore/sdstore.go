package sdstore

import (
	"fmt"
	"github.com/mholt/archiver"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
  "encoding/json"
)

var retryScaler = 1.0

const maxRetries = 6

// SDStore is able to upload, download, and remove the contents of a Reader to the SD Store
type SDStore interface {
	Upload(u *url.URL, filePath string, toCompress bool) error
	Download(url *url.URL) ([]byte, error)
	Remove(url *url.URL) error
}

type sdStore struct {
	token  string
	client *http.Client
}

// NewStore returns an SDStore instance.
func NewStore(token string) SDStore {
	return &sdStore{
		token,
		&http.Client{Timeout: 30 * time.Second},
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
	// add current directory
	filepath = "./" + filepath

	return filepath
}

// Error implements the error interface for SDError
func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

// Remove a file from a path within the SD Store
func (s *sdStore) Remove(u *url.URL) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		_, err := s.remove(u)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file removal: %v", i+1, maxRetries, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("getting from %s after %d retries: %v", u, maxRetries, err)
}

// Download a file from a path within the SD Store
func (s *sdStore) Download(url *url.URL) ([]byte, error) {
	var err error

	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		res, err := s.get(url)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file download: %v", i+1, maxRetries, err)
			continue
		}

		return res, nil
	}

	return nil, fmt.Errorf("getting from %s after %d retries: %v", url, maxRetries, err)
}

func GetMd5Json(path string) (string, error) {
	m, err := MD5All(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	for path := range m {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	jsonString, err := json.Marshal(m)

	if err != nil {
		return nil, err
	}

	return jsonString, nil
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build/event path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdStore) Upload(u *url.URL, filePath string, toCompress bool) error {
	var err error
	md5Json, err := GetMd5Json(filePath)

	if err != nil {
		return fmt.Errorf("Unable to generate md5 of contents, exiting: %v", err)
	}

	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		if toCompress {
			var fileName, zipPath string
			fileName = filepath.Base(filePath)
			zipPath = fmt.Sprintf("%s.zip", fileName)

			err := archiver.Zip.Make(zipPath, []string{filePath})
			if err != nil {
				log.Printf("(Try %d of %d) Unable to zip file: %v", i+1, maxRetries, err)
				continue
			}

			err = s.putFile(u, "application/zip", zipPath, md5Json)
			errRemove := os.Remove(zipPath)

			if err != nil {
				log.Printf("(Try %d of %d) error received from file upload: %v", i+1, maxRetries, err)
				continue
			}

			if errRemove != nil {
				log.Printf("Unable to remove zip file: %v", err)
			}

			return nil
		} else {
			err := s.putFile(u, "text/plain", filePath, md5Json)
			if err != nil {
				log.Printf("(Try %d of %d) error received from file upload: %v", i+1, maxRetries, err)
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("posting to %s after %d retries: %v", filePath, maxRetries, err)
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

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	return handleResponse(res)
}

// GET request from SD Store
func (s *sdStore) get(url *url.URL) ([]byte, error) {
	filePath := getFilePath(url)
	var file *os.File
	var err error

	if filePath != "" {
		dir, _ := filepath.Split(filePath)
		err := os.MkdirAll(dir, 0777)
		file, err = os.Create(filePath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	body, err := handleResponse(res)
	if err != nil {
		return nil, err
	}

	// Write to file
	if filePath != "" {
		_, err := file.Write(body)
		if err != nil {
			return nil, err
		}
	}

	return body, nil
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data from disk to save memory
func (s *sdStore) putFile(url *url.URL, bodyType string, filePath string, md5s string) error {
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
		_, err := s.put(url, bodyType, reader, fsize, md5s)
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
func (s *sdStore) put(url *url.URL, bodyType string, payload io.Reader, size int64, md5s string) ([]byte, error) {
	req, err := http.NewRequest("PUT", url.String(), payload)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	req.Header.Set("X-MD5", md5s)
	req.ContentLength = size

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	return handleResponse(res)
}
