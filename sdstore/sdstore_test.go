package storeinterface

import (
	"bytes"
	"fmt"
	"io"
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
		fmt.Fprintln(w, body)
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

func TestFileUpload(t *testing.T) {
	testBuildID := "testbuild"
	url := "http://fakeurl"
	token := "faketoken"
	testPath := "test/path/1"
	uploader := &sdStore{
		url,
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

		wantURL := fmt.Sprintf("%s/v1/builds/%s/%s", url, testBuildID, testPath)
		if r.URL.String() != wantURL {
			t.Errorf("Wrong URL for the uploader. Got %s, want %s", r.URL.String(), wantURL)
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
	uploader.Upload(testPath, testFile().Name())

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestFileUploadRetry(t *testing.T) {
	retryScaler = .01
	testBuildID := "testbuild"
	url := "http://fakeurl"
	token := "faketoken"
	testPath := "test/path/1"
	uploader := &sdStore{
		url,
		token,
		&http.Client{Timeout: 10 * time.Second},
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	uploader.client = http
	err := uploader.Upload(testPath, testFile().Name())
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if callCount != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}
