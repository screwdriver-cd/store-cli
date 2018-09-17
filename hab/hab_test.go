package hab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

var testHabURL = "http://foo.com/v1/depot"

type ResponseData interface{}

type testData struct {
	packageName   string
	channelName   string
	responses     []ResponseData
	expected      []string
	statusCode    int
	httpError     error
	expectedError error
}

func makeJSONFromResponseData(t *testing.T, responseData ResponseData) string {
	var JSON string

	if pkgsData, ok := responseData.(PackagesInfo); ok {
		bytes, err := json.Marshal(pkgsData)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON for PackagesInfo: %v", err)
		}
		JSON = string(bytes)
	} else if strData, ok := responseData.(string); ok {
		JSON = strData
	} else {
		t.Fatalf("The test data for a request body is strange data type: %v, it expected PackagesInfo or string", reflect.TypeOf(responseData))
	}

	return JSON
}

func makeFakeHTTPClient(t *testing.T, data testData) *http.Client {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resposeJSON := makeJSONFromResponseData(t, data.responses[requestCount])
		requestCount++
		w.WriteHeader(data.statusCode)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, resposeJSON)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			expectedURL := fmt.Sprintf("%s/pkgs/%s", testHabURL, data.packageName)
			if !strings.HasPrefix(req.URL.String(), expectedURL) {
				t.Errorf("Requested URL is %s, but it should start with %s", req.URL, expectedURL)
			}
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func jsonError(JSON string) error {
	return json.Unmarshal([]byte(JSON), nil)
}

func TestPackagesInfoFromName(t *testing.T) {
	tests := []testData{
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 3,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      []string{"0.0.1", "0.0.2", "0.1.0"},
			statusCode:    200,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "unstable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 3,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      nil,
			statusCode:    200,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 5,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "1.0.0",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      []string{"0.0.1", "0.0.2", "0.1.0", "0.1.1", "1.0.0"},
			statusCode:    206,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 8,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "1.0.0",
							Channels: []string{"stable"},
						},
					},
				},
				PackagesInfo{
					RangeStart: 5,
					RangeEnd:   9,
					TotalCount: 8,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "1.0.1",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "1.1.0",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "2.0.0",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      []string{"0.0.1", "0.0.2", "0.1.0", "0.1.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0"},
			statusCode:    206,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 3,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Release:  "20170524100001",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Release:  "20170524100002",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Release:  "20170524100003",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Release:  "20170524100004",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      []string{"0.0.1", "0.0.2", "0.1.0"},
			statusCode:    200,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart: 0,
					RangeEnd:   4,
					TotalCount: 3,
					PackageList: []PackageInfo{
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Release:  "20170524100001",
							Channels: []string{"unstable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.1",
							Release:  "20170524100002",
							Channels: []string{"stable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.0.2",
							Release:  "20170524100003",
							Channels: []string{"unstable"},
						},
						{
							Origin:   "foo",
							Name:     "test",
							Version:  "0.1.0",
							Release:  "20170524100004",
							Channels: []string{"stable"},
						},
					},
				},
			},
			expected:      []string{"0.0.1", "0.1.0"},
			statusCode:    200,
			httpError:     nil,
			expectedError: nil,
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart:  0,
					RangeEnd:    0,
					TotalCount:  0,
					PackageList: []PackageInfo{},
				},
			},
			expected:      nil,
			statusCode:    404,
			httpError:     nil,
			expectedError: errors.New("Package not found"),
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart:  0,
					RangeEnd:    0,
					TotalCount:  0,
					PackageList: []PackageInfo{},
				},
			},
			expected:      nil,
			statusCode:    500,
			httpError:     nil,
			expectedError: errors.New("Unexpected status code: 500"),
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				PackagesInfo{
					RangeStart:  0,
					RangeEnd:    0,
					TotalCount:  0,
					PackageList: []PackageInfo{},
				},
			},
			expected:      nil,
			statusCode:    500,
			httpError:     nil,
			expectedError: errors.New("Unexpected status code: 500"),
		},
		{
			packageName: "foo/test",
			channelName: "stable",
			responses: []ResponseData{
				"corrupted json data",
			},
			expected:      nil,
			statusCode:    200,
			httpError:     nil,
			expectedError: jsonError("corrupted json data"),
		},
	}

	for _, test := range tests {
		http := makeFakeHTTPClient(t, test)
		testDepot := &depot{testHabURL, http}

		results, err := testDepot.PackageVersionsFromName(test.packageName, test.channelName)

		if test.expectedError == nil && err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if test.expectedError != nil {
			if reflect.TypeOf(err) != reflect.TypeOf(test.expectedError) {
				t.Fatalf("Expected error type: %v, actual %v", reflect.TypeOf(test.expectedError), reflect.TypeOf(err))
			} else if err.Error() != test.expectedError.Error() {
				t.Errorf("Expected error message %v, actual %v", test.expectedError, err)
			}
		} else {
			if !reflect.DeepEqual(results, test.expected) {
				t.Errorf("Expected versions %v, actual %v", test.expected, results)
			}
		}
	}
}
