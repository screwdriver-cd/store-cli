package sdstore

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/mholt/archiver"
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

var retryScaler = 1.0

const maxRetries = 3

// SDStore is able to upload, download, and remove the contents of a Reader to the SD Store
type SDStore interface {
	Upload(u *url.URL, filePath string, toCompress bool) error
	Download(url *url.URL, toExtract bool) ([]byte, error)
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
	return fmt.Errorf("removing from %s after %d retries: %v", u, maxRetries, err)
}

// Download a file from a path within the SD Store
func (s *sdStore) Download(url *url.URL, toExtract bool) ([]byte, error) {
	var err error

	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		res, err := s.get(url, toExtract)
		if err != nil {
			log.Printf("(Try %d of %d) error received from file download: %v", i+1, maxRetries, err)
			continue
		}

		return res, nil
	}

	return nil, fmt.Errorf("getting from %s after %d retries: %v", url, maxRetries, err)
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

	for i := 0; i < maxRetries; i++ {
		time.Sleep(time.Duration(float64(i*i)*retryScaler) * time.Second)

		if toCompress {
			var fileName string
			fileName = filepath.Base(filePath)
			encodedURL, _ := url.Parse(fmt.Sprintf("%s%s", u.String(), "_md5.json"))
			md5Json, err := s.GenerateAndCheckMd5Json(encodedURL, filePath)

			if err != nil && err.Error() == "Contents unchanged" {
				log.Printf("No change to %d, aborting upload", filePath)
				return nil
			}

			if err != nil {
				log.Printf("(Try %d of %d) error received from generating md5: %v", i+1, maxRetries, err)
				continue
			}

			err = s.putFile(encodedURL, "application/json", md5Json)
			if err != nil {
				log.Printf("(Try %d of %d) error received from uploading md5 json: %v", i+1, maxRetries, err)
				continue
			}

			err = os.Remove(md5Json)
			if err != nil {
				log.Printf("Unable to remove md5 file from path: %s, continuing", md5Json)
			}

			var zipPath string
			zipPath, err = filepath.Abs(fmt.Sprintf("%s.zip", fileName))

			if err != nil {
				log.Printf("(Try %d of %d) Unable to determine filepath: %v", i+1, maxRetries, err)
				continue
			}

			absPath, _ := filepath.Abs(filePath)
			err = archiver.Zip.Make(zipPath, []string{absPath})
			if err != nil {
				log.Printf("(Try %d of %d) Unable to zip file: %v", i+1, maxRetries, err)
				continue
			}

			encodedURL, _ = url.Parse(fmt.Sprintf("%s%s", u.String(), ".zip"))
			err = s.putFile(encodedURL, "text/plain", zipPath)
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
			err := s.putFile(u, "text/plain", filePath)
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

// Taken from https://golangcode.com/unzip-files-in-go/
func Unzip(src string, dest string) ([]string, error) {
	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}
		defer rc.Close()

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if dest != "/" && !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {

			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)

		} else {

			// Make File
			if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return filenames, err
			}

			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return filenames, err
			}

			_, err = io.Copy(outFile, rc)

			// Close the file without defer to close before next iteration of loop
			outFile.Close()

			if err != nil {
				return filenames, err
			}

		}
	}
	return filenames, nil
}
