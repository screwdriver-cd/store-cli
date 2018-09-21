package sdstore

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var retryScaler = 1.0

const maxRetries = 6

// SDStore is able to upload, download, and remove the contents of a Reader to the SD Store
type SDStore interface {
	Upload(u *url.URL, filePath string) error
	Download(url *url.URL) error
}

type sdStore struct {
	token   string
	client  *http.Client
}

// NewStore returns an SDStore for a given url.
func NewStore(url, token string) SDStore {
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

// Error implements the error interface for SDError
func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

// Remove a file from a path within the SD Store
func (s *sdStore) Remove(url *url.URL) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		_, err := s.remove(url)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file removal: %v", i+1, maxRetries, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("getting from %s after %d retries: %v", url, maxRetries, err)
}

// Download a file from a path within the SD Store
func (s *sdStore) Download(url *url.URL) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		_, err := s.get(url)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file download: %v", i+1, maxRetries, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("getting from %s after %d retries: %v", url, maxRetries, err)
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build/event path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdStore) Upload(u *url.URL, filePath string) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		err := s.putFile(u, "application/x-ndjson", filePath)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file upload: %v", i+1, maxRetries, err)
			continue
		}
		return nil
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

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
}

// GET request from SD Store
func (s *sdStore) get(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
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

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
}
