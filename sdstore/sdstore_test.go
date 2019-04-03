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
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newStore(retryScaler float64, maxRetries int) *sdStore {
	return &sdStore{
		token:       "faketoken",
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: retryScaler,
		maxRetries:  maxRetries,
	}
}

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

		if r.URL.String() == "http://fakestore.example.com/v1/caches/events/123/../data/emitterdata_md5.json" {
			w.Write([]byte("{\"../data/emitterdata\":\"73a256001a246e77fd1941ca007b50r2\"}"))
		} else if r.URL.String() == "http://fakestore.example.com/v1/caches/events/123/../data/emitterdata2_md5.json" {
			w.Write([]byte("{\"../data/emitterdata2\":\"b567651333fff804168aabac8284d708\"}"))
		} else {
			w.Write([]byte(body))
		}
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func makeFakeZipHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)(r)
		if v != nil {
			v(r)
		}

		w.WriteHeader(code)
		w.Header().Set("Content-Type", "text/plain")
		filePath, _ := filepath.Abs("../data/test.zip")
		fileContent, _ := ioutil.ReadFile(filePath)
		w.Write(fileContent)
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

func testFile2() *os.File {
	f, err := os.Open("../data/emitterdata2")
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
	u, _ := url.Parse("http://fakestore.example.com/builds/emitterdata")
	uploader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3,
	}
	called := false

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		if r.Method == "PUT" {
			called = true
			got := bytes.NewBuffer(nil)
			io.Copy(got, r.Body)
			r.Body.Close()

			if got.String() != want.String() {
				t.Errorf("Received payload %s, want %s", got, want)
			}

			stat, err := testFile().Stat()
			if err != nil {
				t.Fatalf("Couldn't stat test file: %v", err)
			}

			fsize := stat.Size()
			if r.ContentLength != fsize {
				t.Errorf("Wrong Content-Length sent to uploader. Got %d, want %d", r.ContentLength, fsize)
			}
		} else if r.Method == "GET" {
		}
	})
	uploader.client = http
	uploader.Upload(u, testFile().Name(), false)

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestUploadZipWithChange(t *testing.T) {
	token := "faketoken"
	file := "../data/emitterdata"
	zipfile := file + ".zip"
	u, _ := url.Parse("http://fakestore.example.com/v1/caches/events/123/" + file)
	uploader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := 0
	getMd5 := false
	putZip := false
	putMd5 := false

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()
	wantcontent, _ := ioutil.ReadAll(want)

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		called++
		got := bytes.NewBuffer(nil)
		io.Copy(got, r.Body)
		r.Body.Close()

		content, _ := ioutil.ReadAll(got)

		if r.Method == "GET" {
			getMd5 = true
		} else if r.Method == "PUT" && contentType == "text/plain" {
			putZip = true
			err := ioutil.WriteFile(zipfile, content, 0644)
			if err != nil {
				panic(err)
			}

			files, err := Unzip(zipfile, "../data/test")

			var filecontent []byte
			if len(files) == 1 {
				filecontent, err = ioutil.ReadFile(files[0])
			}

			if string(filecontent[:]) != string(wantcontent[:]) {
				t.Errorf("Received payload %s, want %s", filecontent, wantcontent)
			}

			stat, err := testZipFile().Stat()
			if err != nil {
				t.Fatalf("Couldn't stat test file: %v", err)
			}

			fsize := stat.Size()
			if r.ContentLength != fsize {
				t.Errorf("Wrong Content-Length sent to uploader. Got %d, want %d", r.ContentLength, fsize)
			}

			err = os.Remove(zipfile)
			if err != nil {
				panic(err)
			}

			err = os.RemoveAll("../data/test")
			if err != nil {
				panic(err)
			}
		} else if r.Method == "PUT" && contentType == "application/json" {
			putMd5 = true
			md5Json, _ := ioutil.ReadFile("emitterdata_md5.json")
			wantmd5 := fmt.Sprintf("{\"" + file + "\":\"62a256001a246e77fd1941ca007b50e1\"}")

			if string(md5Json) != wantmd5 {
				t.Errorf("Expected content of md5 json to be %s, got %s", md5Json, wantmd5)
			}
		} else if r.Method == "PUT" {
			t.Errorf("Wrong content type, expected one of text/plain or application/json")
		}
	})

	uploader.client = http
	uploader.Upload(u, testFile().Name(), true)

	if !getMd5 {
		t.Errorf("Did not get md5 file")
	}

	if !putZip {
		t.Errorf("Did not upload zip file")
	}

	if !putMd5 {
		t.Errorf("Did not upload md5 file")
	}

	if called != 3 { // 1 GET, 2 PUTs
		t.Fatalf("The HTTP client was not called as expected")
	}
}

func TestUploadZipNoChange(t *testing.T) {
	token := "faketoken"
	file := "../data/emitterdata2"
	u, _ := url.Parse("http://fakestore.example.com/v1/caches/events/123/" + file)
	uploader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := 0

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()
	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		called++
		got := bytes.NewBuffer(nil)
		io.Copy(got, r.Body)
		r.Body.Close()

		if r.Method == "PUT" && contentType == "text/plain" {
			t.Errorf("Should not put zip file")
		} else if r.Method == "PUT" && contentType == "application/json" {
			t.Errorf("Should not put Md5 file")
		}
	})

	uploader.client = http
	uploader.Upload(u, testFile2().Name(), true)

	if called != 1 { // 1 GET
		t.Fatalf("The HTTP client was not called as expected")
	}
}

func TestUploadRetry(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	uploader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: .01,
		maxRetries:  3.0,
	}

	callCount := int32(0)
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		atomic.AddInt32(&callCount, 1)
	})
	uploader.client = http
	err := uploader.Upload(u, testFile().Name(), false)
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("Expected 3 retries, got %d", callCount)
	}
}

func TestUploadZipRetry(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	uploader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: .01,
		maxRetries:  3.0,
	}

	callCount := int32(0)
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		os.Remove("emitterdata_md5.json")
	})
	uploader.client = http
	err := uploader.Upload(u, testFile().Name(), true)
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if atomic.LoadInt32(&callCount) != 6 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestDownload(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	downloader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, want, func(r *http.Request) {
		if r.URL.Path != u.Path {
			t.Errorf("Wrong URL path: %s", r.URL.Path)
		}

		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u, false)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestDownloadZip(t *testing.T) {
	token := "faketoken"
	abspath, _ := filepath.Abs("./")
	testfilepath := abspath + "/../data/test.zip"
	testfilepath = url.QueryEscape(testfilepath)

	u, _ := url.Parse("http://fakestore.example.com/v1/caches/events/1234/" + testfilepath)
	downloader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := false

	http := makeFakeZipHTTPClient(t, 200, "OK", func(r *http.Request) {
		if r.URL.Path != fmt.Sprintf("%s%s", u.Path, ".zip") {
			t.Errorf("Wrong URL path, needs to be a zip file: %s", r.URL.Path)
		}

		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	_, _ = downloader.Download(u, true)

	want, _ := ioutil.ReadFile("../data/emitterdata")
	got, _ := ioutil.ReadFile(abspath + "/../data/tmp/test/emitterdata")

	err := os.RemoveAll("/tmp/test")

	if err != nil {
		panic(err)
	}

	if string(got) != string(want) {
		t.Errorf("Response is %s, want %s", got, want)
	}

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestDownloadRetry(t *testing.T) {
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	downloader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: .01,
		maxRetries:  3.0,
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	downloader.client = http
	_, err := downloader.Download(u, false)
	if err == nil {
		t.Error("Expected error from downloader.Download(), got nil")
	}
	if callCount != 3 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestDownloadWriteBack(t *testing.T) {
	token := "faketoken"
	testfilepath := "/tmp/test-data/node_modules/schema/file"
	u, _ := url.Parse("http://fakestore.example.com/v1/caches/events/1234/" + testfilepath)
	downloader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, want, func(r *http.Request) {
		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u, false)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	filecontent, err := ioutil.ReadFile(testfilepath)
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
	testfolder := "/tmp/test-data/node_modules/schema/"
	testfilename := "!-_.*'()&@:,.$= ?; space"
	u, _ := url.Parse("http://fakestore.example.com/v1/caches/events/1234/" + testfolder + "%21-_.%2A%27%28%29%26%40%3A%2C.%24%3D%2B%3F%3B+space")
	downloader := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
	}
	called := false

	want := "test-content"

	http := makeFakeHTTPClient(t, 200, want, func(r *http.Request) {
		called = true

		if r.Method != "GET" {
			t.Errorf("Called with method %s, want GET", r.Method)
		}
	})

	downloader.client = http
	res, _ := downloader.Download(u, false)

	if string(res) != want {
		t.Errorf("Response is %s, want %s", string(res), want)
	}

	fileInfo, err := os.Stat(testfolder + testfilename)
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
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	removeRes := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: 1.0,
		maxRetries:  3.0,
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
	token := "faketoken"
	u, _ := url.Parse("http://fakestore.example.com/builds/1234-test")
	removeRes := &sdStore{
		token:       token,
		client:      &http.Client{Timeout: 10 * time.Second},
		retryScaler: .01,
		maxRetries:  3.0,
	}

	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	removeRes.client = http
	err := removeRes.Remove(u)
	if err == nil {
		t.Errorf("Expected error from removeRes.Remove(), got nil")
	}
	if callCount != 3 {
		t.Errorf("Expected 6 retries, got %d", callCount)
	}
}

func TestZipAndUnzipWithSymlink(t *testing.T) {
	err := Zip("../data/testsymlink", "../data/testsymlink.zip")

	if err != nil {
		t.Errorf("Unable to zip file")
	}

	_, err = Unzip("../data/testsymlink.zip", "../data/test")

	if err != nil {
		t.Errorf("Unable to unzip file %v", err)
	}

	fi, err := os.Readlink("../data/test/testsymlink/symlink")
	if err != nil {
		t.Errorf("Could not read symbolic link: %v", err)
	}

	if fi != "bar/test" {
		t.Errorf("Expected symlink to point to bar/test, got %s", fi)
	}

	os.RemoveAll("../data/test")
	os.RemoveAll("../data/testsymlink.zip")
}

func TestBackOff(t *testing.T) {
	client := newStore(1.5, 5)
	cases := []struct {
		attemptNum  int
		backoffTime time.Duration
	}{
		{attemptNum: 1, backoffTime: 1 * time.Second},
		{attemptNum: 5, backoffTime: 37 * time.Second},
	}

	for _, c := range cases {
		bt := client.backOff(c.attemptNum)
		assert.Equal(t, c.backoffTime, bt, fmt.Sprintf("when attempt number is %v", c.attemptNum))
		if bt != c.backoffTime {
			t.Errorf("Expected time is %q. But Got %q", c.backoffTime, bt)
		}
	}
}

func TestCheckForRetry(t *testing.T) {
	client := newStore(1.0, 3)

	cases := []struct {
		err        error
		statusCode int
		doesRetry  bool
		retryErr   error
	}{
		{err: fmt.Errorf("awful things happen"), statusCode: 0, doesRetry: true, retryErr: fmt.Errorf("awful things happen")},
		// statut 200~
		{statusCode: http.StatusOK, doesRetry: false},
		{statusCode: http.StatusCreated, doesRetry: false},
		{statusCode: http.StatusAccepted, doesRetry: false},
		{statusCode: http.StatusNoContent, doesRetry: false},
		// status 400~
		{statusCode: http.StatusBadRequest, doesRetry: true, retryErr: fmt.Errorf("got code 400.")},
		{statusCode: http.StatusUnauthorized, doesRetry: true, retryErr: fmt.Errorf("got code 401.")},
		{statusCode: http.StatusForbidden, doesRetry: true, retryErr: fmt.Errorf("got code 403.")},
		{statusCode: http.StatusNotFound, doesRetry: false, retryErr: fmt.Errorf("got code 404. stop retrying")},
		{statusCode: http.StatusRequestTimeout, doesRetry: true, retryErr: fmt.Errorf("got code 408.")},
		// status 500~
		{err: nil, statusCode: http.StatusInternalServerError, doesRetry: true, retryErr: fmt.Errorf("got code 500.")},
	}

	for _, c := range cases {
		res := &http.Response{}
		res.StatusCode = c.statusCode
		res.Status = fmt.Sprintf("code %d", res.StatusCode)
		retry, err := client.checkForRetry(res, c.err)
		assert.Equal(t, c.doesRetry, retry, fmt.Sprintf("when status is %d and err is %q", c.statusCode, c.err))
		assert.Equal(t, c.retryErr, err, "when status is %d and err is %q", c.statusCode, c.err)
	}
}

func TestDo(t *testing.T) {
	client := newStore(.01, 4)
	req, _ := http.NewRequest("GET", "http://fakestore.example.com/v1/test", nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", client.token))

	cases := []struct {
		statusCode  int
		body        string
		callCount   int
		err         error
		responseNil bool
	}{
		{statusCode: 500, body: "ERROR", callCount: 4, err: fmt.Errorf("getting error from http://fakestore.example.com/v1/test after 4 retries: got 500 Internal Server Error."), responseNil: true},
		{statusCode: 200, body: "test-contents", callCount: 1},
		{statusCode: 404, body: "Not Found", callCount: 1, err: fmt.Errorf("got 404 Not Found from http://fakestore.example.com/v1/test. stop retrying")},
	}

	for _, c := range cases {
		when := fmt.Sprintf("when statusCode %d and body %s", c.statusCode, c.body)
		callCount := 0
		client.client = makeFakeHTTPClient(t, c.statusCode, c.body, func(r *http.Request) {
			callCount++
		})
		res, err := client.do(req)

		assert.Equal(t, c.callCount, callCount, when)
		assert.Exactly(t, c.err, err, when)
		if c.responseNil {
			assert.Nil(t, res, when)
		} else {
			b, _ := ioutil.ReadAll(res.Body)
			assert.Equal(t, c.body, string(b), when)
			res.Body.Close()
		}
	}
}
