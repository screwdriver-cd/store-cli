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
	os.Setenv("SD_STORE_URL", "http://store.screwdriver.cd")
	os.Setenv("SD_BUILD_ID", "10038")
	os.Setenv("SD_EVENT_ID", "499")

	storeType := "cache"
	scope := "events"
	key := "cache-1"
	i, _ := makeURL(storeType, scope, key)
	expected := "http://store.screwdriver.cd/v1/caches/events/499/cache-1"
	if i.String() != expected {
		t.Fatalf("Expected '%s' but '%s'", expected, i)
	}

	storeType = "artifacts"
	scope = "builds"
	key = "test"
	i, _ = makeURL(storeType, scope, key)
	expected = "http://store.screwdriver.cd/v1/builds/10038-test"
	if i.String() != expected {
		t.Fatalf("Expected '%s' but '%s'", expected, i)
	}

	storeType = "artifacts"
	scope = "events"
	key = "test"
	i, _ = makeURL(storeType, scope, key)
	expected = "http://store.screwdriver.cd/v1/builds/events/499/test"
	if i.String() != expected {
		t.Fatalf("Expected '%s' but '%s'", expected, i)
	}

	storeType = "logs"
	scope = "pipelines"
	key = "test2"
	i, _ = makeURL(storeType, scope, key)
	expected = "http://store.screwdriver.cd/v1"
	if i.String() != expected {
		t.Fatalf("Expected '%s' but '%s'", expected, i)
	}
}
