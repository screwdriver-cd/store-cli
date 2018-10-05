package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// run test
	retCode := m.Run()
	// teardown functions
	os.Exit(retCode)
}

func TestMakeURL(t *testing.T) {
	os.Setenv("SD_STORE_URL", "http://store.screwdriver.cd/v1/")
	os.Setenv("SD_BUILD_ID", "10038")
	os.Setenv("SD_EVENT_ID", "499")
	os.Setenv("SD_PIPELINE_ID", "100")

	// Success test cases
	testCases := []struct {
		storeType string
		scope     string
		key       string
		expected  string
	}{
		{"cache", "events", "cache-1", "http://store.screwdriver.cd/v1/caches/events/499/cache-1"},
		{"cache", "events", "./cache-1", "http://store.screwdriver.cd/v1/caches/events/499/cache-1"},
		{"cache", "events", "/tmp/test/1/2/3/4/", "http://store.screwdriver.cd/v1/caches/events/499/%2Ftmp%2Ftest%2F1%2F2%2F3%2F4"},
		{"cache", "events", "./!-_.*'()&@:,.$=+?; space", "http://store.screwdriver.cd/v1/caches/events/499/%21-_.%2A%27%28%29%26%40%3A%2C.%24%3D%2B%3F%3B+space"},
		{"artifacts", "events", "artifact-1", "http://store.screwdriver.cd/v1/builds/10038-ARTIFACTS/artifact-1"},
		{"artifacts", "builds", "test", "http://store.screwdriver.cd/v1/builds/10038-ARTIFACTS/test"},
		{"logs", "builds", "testlog", "http://store.screwdriver.cd/v1/builds/10038-testlog"},
		{"logs", "builds", "step-bookend", "http://store.screwdriver.cd/v1/builds/10038-step-bookend"},
		{"logs", "pipelines", "test-2", "http://store.screwdriver.cd/v1/builds/10038-test-2"},
	}

	for _, tc := range testCases {
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
