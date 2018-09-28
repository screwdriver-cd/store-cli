package sdstore

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
	uploader.Upload(u, testFile().Name())

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
	err := uploader.Upload(u, testFile().Name())
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
	testfilepath := "test-data/node_modules/schema/!-_.*'()&@:,.$=+?; space"
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
