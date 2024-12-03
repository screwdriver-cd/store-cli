package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// run test
	retCode := m.Run()
	// teardown functions
	os.Exit(retCode)
}

func TestSkipCache(t *testing.T) {
	// Success test cases
	testCases := []struct {
		storeType string
		scope     string
		action    string
		prNum     string
		expected  bool
	}{
		{"cache", "pipeline", "get", "", false},
		{"cache", "pipeline", "get", "123", false},
		{"cache", "pipeline", "set", "123", true},
		{"cache", "pipeline", "remove", "123", true},
		{"cache", "event", "get", "", false},
		{"cache", "job", "get", "", false},
		{"cache", "job", "set", "123", true},
		{"artifact", "event", "get", "", false},
		{"log", "build", "set", "123", false},
	}

	for _, tc := range testCases {
		os.Setenv("SD_PULL_REQUEST", tc.prNum)
		skip := skipCache(tc.storeType, tc.scope, tc.action)
		if skip != tc.expected {
			t.Fatalf("%s %s for scope %s, expected '%t' got '%t'", tc.action, tc.storeType, tc.scope, tc.expected, skip)
		}
	}
}

func TestMakeURL(t *testing.T) {
	os.Setenv("SD_STORE_URL", "http://store.screwdriver.cd/v1/")
	os.Setenv("SD_BUILD_ID", "10038")
	os.Setenv("SD_JOB_ID", "888")
	os.Setenv("SD_EVENT_ID", "499")
	os.Setenv("SD_PIPELINE_ID", "100")
	abspath, _ := filepath.Abs("./")
	abspath = url.PathEscape(abspath)

	// Success test cases
	testCases := []struct {
		storeType string
		scope     string
		key       string
		expected  string
	}{
		{"cache", "job", "/myprcache", "http://store.screwdriver.cd/v1/caches/jobs/987/%2Fmyprcache"},
		{"cache", "event", "/mycache", "http://store.screwdriver.cd/v1/caches/events/499/%2Fmycache"},
		{"cache", "event", "mycache", fmt.Sprintf("%s%s", "http://store.screwdriver.cd/v1/caches/events/499/", "mycache")},
		{"cache", "event", "./mycache", fmt.Sprintf("%s%s", "http://store.screwdriver.cd/v1/caches/events/499/", "mycache")},
		{"cache", "event", "/tmp/mycache/1/2/3/4/", "http://store.screwdriver.cd/v1/caches/events/499/%2Ftmp%2Fmycache%2F1%2F2%2F3%2F4"},
		{"cache", "event", "/!-_.*'()&@:,.$=+?; space", "http://store.screwdriver.cd/v1/caches/events/499/%2F%21-_.%2A%27%28%29&@:%2C.$=+%3F%3B%20space"},
		{"artifact", "event", "artifact-1", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/artifact-1"},
		{"artifact", "build", "test", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/test"},
		{"artifact", "", ".test", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/.test"},
		{"artifact", "", "./test", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/test"},
		{"artifact", "", "test/foo", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/test%2Ffoo"},
		{"artifact", "", "test/foo./bar", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/test%2Ffoo.%2Fbar"},
		{"artifact", "", "test/foo/あいうえお.txt", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/test%2Ffoo%2F%E3%81%82%E3%81%84%E3%81%86%E3%81%88%E3%81%8A.txt"},
		{"artifact", "", "!-_.*'()&@:,.$=+?; space", "http://store.screwdriver.cd/v1/builds/10038/ARTIFACTS/%21-_.%2A%27%28%29&@:%2C.$=+%3F%3B%20space"},
		{"log", "build", "testlog", "http://store.screwdriver.cd/v1/builds/10038-testlog"},
		{"log", "build", "step-bookend", "http://store.screwdriver.cd/v1/builds/10038-step-bookend"},
		{"log", "pipeline", "test-2", "http://store.screwdriver.cd/v1/builds/10038-test-2"},
	}

	for _, tc := range testCases {
		if tc.key == "/myprcache" {
			os.Setenv("SD_PULL_REQUEST", "900")
			os.Setenv("SD_PR_PARENT_JOB_ID", "987")
		}
		i, _ := makeURL(tc.storeType, tc.scope, tc.key)
		if i.String() != tc.expected {
			t.Fatalf("Expected '%s' got '%s'", tc.expected, i)
		}
	}

	// Error test case
	var err error
	storeType := "invalid"
	scope := "pipelines"
	key := "test2"
	_, err = makeURL(storeType, scope, key)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestGetTimeout(t *testing.T) {
	testCases := []struct {
		name           string
		flagTimeout    string
		envName        string
		defaultTimeout int
		expected       int
		envValue       string
		shouldError    bool
	}{
		{
			name:           "use flag timeout",
			flagTimeout:    "50",
			envName:        "SD_STORE_CLI_DOWNLOAD_HTTP_TIMEOUT",
			defaultTimeout: 60,
			expected:       50,
			envValue:       "",
			shouldError:    false,
		},
		{
			name:           "use environment timeout",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_DOWNLOAD_HTTP_TIMEOUT",
			defaultTimeout: 60,
			expected:       70,
			envValue:       "70",
			shouldError:    false,
		},
		{
			name:           "use default upload timeout",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT",
			defaultTimeout: UPLOAD_HTTP_TIMEOUT,
			expected:       60,
			envValue:       "",
			shouldError:    false,
		},
		{
			name:           "use default download timeout",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_DOWNLOAD_HTTP_TIMEOUT",
			defaultTimeout: DOWNLOAD_HTTP_TIMEOUT,
			expected:       300,
			envValue:       "",
			shouldError:    false,
		},
		{
			name:           "use default remove timeout",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_REMOVE_HTTP_TIMEOUT",
			defaultTimeout: REMOVE_HTTP_TIMEOUT,
			expected:       300,
			envValue:       "",
			shouldError:    false,
		},
		{
			name:           "set flagTimeout to zero",
			flagTimeout:    "0",
			envName:        "SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT",
			defaultTimeout: UPLOAD_HTTP_TIMEOUT,
			expected:       0,
			envValue:       "",
			shouldError:    false,
		},
		{
			name:           "set envValue to zero",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT",
			defaultTimeout: UPLOAD_HTTP_TIMEOUT,
			expected:       0,
			envValue:       "0",
			shouldError:    false,
		},
		{
			name:           "Error case set flagTimeout to string",
			flagTimeout:    "dummystring",
			envName:        "SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT",
			defaultTimeout: UPLOAD_HTTP_TIMEOUT,
			expected:       0,
			envValue:       "",
			shouldError:    true,
		},
		{
			name:           "Error case set envValue to string",
			flagTimeout:    "",
			envName:        "SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT",
			defaultTimeout: UPLOAD_HTTP_TIMEOUT,
			expected:       0,
			envValue:       "dummystring",
			shouldError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue != "" {
				os.Setenv(tc.envName, tc.envValue)
			}

			got, err := getTimeout(tc.flagTimeout, tc.envName, tc.defaultTimeout)

			if err != nil {
				if tc.shouldError {
					return
				}
				t.Fatalf("getTimeout() error = %v", err)
				return
			}

			// If we reach here, the test get error.
			if tc.shouldError {
				t.Fatal("getTimeout() expected an error, but got nil")
			}

			if got != tc.expected {
				t.Fatalf("getTimeout() got = %v, want %v", got, tc.expected)
			}

			// Clear environment value for the next test case.
			os.Unsetenv(tc.envName)
		})
	}
}

func TestIsEnableExpectHeader(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run("env:"+tt.envValue, func(t *testing.T) {
			os.Setenv("SD_ENABLE_EXPECT_HEADER", tt.envValue)

			result := IsEnableExpectHeader()

			if result != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, result)
			}
		})
	}

	os.Unsetenv("SD_ENABLE_EXPECT_HEADER")
}
