package sdstore

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validateHeader(t *testing.T, key, value string) func(r *http.Request) {
	return func(r *http.Request) {
		headers, ok := r.Header[key]
		if !ok {
			t.Fatalf("No %s header sent in Screwdriver request", key)
		}
		header := headers[0]
		if header != value {
			t.Errorf("%s header = %q, want %q", key, header, value)
		}
	}
}

func makeFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)(r)
		if v != nil {
			v(r)
		}

		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("test-content"))
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func testFile() *os.File {
	f, err := os.Open("../data/emitterdata")
	if err != nil {
		panic(err)
	}
	return f
}

func testZipFile() *os.File {
	f, err := os.Open("../data/emitterdata.zip")
	if err != nil {
		panic(err)
	}
	return f
}

func TestUpload(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	uploader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true
		got := bytes.NewBuffer(nil)
		io.Copy(got, r.Body)
		r.Body.Close()

		if got.String() != want.String() {
			t.Errorf("Received payload %s, want %s", got, want)
		}

		if r.Method != "PUT" {
			t.Errorf("Uploaded with method %s, want PUT", r.Method)
		}

		stat, err := testFile().Stat()
		if err != nil {
			t.Fatalf("Couldn't stat test file: %v", err)
		}

		fsize := stat.Size()
		if r.ContentLength != fsize {
			t.Errorf("Wrong Content-Length sent to uploader. Got %d, want %d", r.ContentLength, fsize)
		}
	})
	uploader.client = http
	uploader.Upload(u, testFile().Name(), false)

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestUploadZip(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	uploader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()
	wantcontent, _ := ioutil.ReadAll(want)

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true
		got := bytes.NewBuffer(nil)
		io.Copy(got, r.Body)
		r.Body.Close()

		content, _ := ioutil.ReadAll(got)

		err := ioutil.WriteFile("../data/emitterdata.zip", content, 0644)
		if err != nil {
			panic(err)
		}

		files, err := Unzip("../data/emitterdata.zip", "../data/test")

		var filecontent []byte
		if len(files) == 1 {
			filecontent, err = ioutil.ReadFile(files[0])
		}

		if string(filecontent[:]) != string(wantcontent[:]) {
			t.Errorf("Received payload %s, want %s", filecontent, wantcontent)
		}

		if r.Method != "PUT" {
			t.Errorf("Uploaded with method %s, want PUT", r.Method)
		}

		stat, err := testZipFile().Stat()
		if err != nil {
			t.Fatalf("Couldn't stat test file: %v", err)
		}

		fsize := stat.Size()
		if r.ContentLength != fsize {
			t.Errorf("Wrong Content-Length sent to uploader. Got %d, want %d", r.ContentLength, fsize)
		}

		if r.Header.Get("Content-Type") != "application/zip" {
			t.Errorf("Wrong Content-Type sent to uploader. Got %s, want application/zip", r.Header.Get("Content-Type"))
		}

		err = os.Remove("../data/emitterdata.zip")
		if err != nil {
			panic(err)
		}

		err = os.RemoveAll("../data/test")
		if err != nil {
			panic(err)
		}
	})
	uploader.client = http
	uploader.Upload(u, testFile().Name(), true)

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestUploadRetry(t *testing.T) {
	retryScaler = .01
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	uploader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	uploader.client = http
	err := uploader.Upload(u, testFile().Name(), false)
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if callCount != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestUploadZipRetry(t *testing.T) {
	retryScaler = .01
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	uploader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	uploader.client = http
	err := uploader.Upload(u, testFile().Name(), true)
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if callCount != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestDownload(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	downloader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestDownloadRetry(t *testing.T) {
	retryScaler = .01
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	downloader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	downloader.client = http
	_, err := downloader.Download(u)
	if err == nil {
		t.Error("Expected error from downloader.Download(), got nil")
	}
	if callCount != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestDownloadWriteBack(t *testing.T) {
	token := "faketoken"
	testfilepath := "test-data/node_modules/schema/file"
	u, _ := url.Parse("http://fakestore.com/v1/caches/events/1234/" + testfilepath)
	downloader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	filecontent, err := ioutil.ReadFile("./" + testfilepath)
	if err != nil {
		t.Errorf("File content is not written")
	}

	if string(filecontent) != want {
		t.Errorf("File content is %s, want %s", string(filecontent), want)
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestDownloadWriteBackSpecialFile(t *testing.T) {
	token := "faketoken"
	testfolder := "./test-data/node_modules/schema/"
	u, _ := url.Parse("http://fakestore.com/v1/caches/events/1234/test-data/node_modules/schema/%21-_.%2A%27%28%29%26%40%3A%2C.%24%3D%2B%3F%3B+space")
	downloader := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	fileInfo, err := os.Stat(testfolder + "!-_.*'()&@:,.$= ?; space")
	filecontent, err := ioutil.ReadFile(testfolder + fileInfo.Name())
	if err != nil {
		t.Errorf("File content is not written")
	}

	if string(filecontent) != want {
		t.Errorf("File content is %s, want %s", string(filecontent), want)
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestRemove(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	removeRes := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
	called := false

	http := makeFakeHTTPClient(t, 202, "OK", func(r *http.Request) {
		called = true

		if r.Method != "DELETE" {
			t.Errorf("Called with method %s, want DELETE", r.Method)
		}
	})

	removeRes.client = http
	err := removeRes.Remove(u)

	if err != nil {
		t.Error("Expected nil from removeRes.Remove(), got error")
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestRemoveRetry(t *testing.T) {
	retryScaler = .01
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.com/builds/1234-test")
	removeRes := &sdStore{
		token,
		&http.Client{Timeout: 10 * time.Second},
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	removeRes.client = http
	err := removeRes.Remove(u)
	if err == nil {
		t.Error("Expected error from removeRes.Remove(), got nil")
	}
	if callCount != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
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
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
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
