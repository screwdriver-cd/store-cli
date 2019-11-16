package sdstore

import (
	"testing"
	"os"
	"gotest.tools/assert"
	"path/filepath"
	"io/ioutil"
)

func Setup() {
	os.Setenv("SD_PIPELINE_CACHE_DIR", "../data/cache/pipeline")
	os.Setenv("SD_JOB_CACHE_DIR", "../data/cache/job")
	os.Setenv("SD_EVENT_CACHE_DIR", "../data/cache/event")
}

// test to validate invalid command
func TestCache2DiskInvalidCommand(t *testing.T) {
	err := Cache2Disk("", "pipeline", "")
	assert.Assert(t, err.Error() == "Error: <nil>, command:  is not expected")
}

// test to validate invalid cache scope
func TestCache2DiskInvalidCacheScope(t *testing.T) {
	err := Cache2Disk("set", "", "")
	assert.Assert(t, err.Error() == "Error: <nil>, cache directory empty for cache scope ")
}

// test to validate invalid src and cache path
func TestCache2DiskInvalidSrcPath(t *testing.T) {
	Setup()
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local")
	assert.ErrorContains(t, err, "no such file or directory")
}

// test to remove cache files from shared storage pipeline directory
func TestCache2DiskRemoveCache(t *testing.T) {
	Setup()
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("remove", "pipeline", "../data/cache/local") == nil)

	_, err := os.Stat(filepath.Join(cache,local))
	assert.ErrorContains(t, err, "no such file or directory")
}

// test to copy cache files from local build dir to shared storage
// pipeline directory
func TestCache2DiskForPipeline(t *testing.T) {
	Setup()
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "pipeline", local) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)
}

// test to copy cache files from local build dir to shared storage
// job directory
func TestCache2DiskForJob(t *testing.T) {
	Setup()
	cache, _ := filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "job", local) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	defer os.RemoveAll("../data/cache/job/")
}

// test to copy cache files from local build dir to shared storage
// event directory
func TestCache2DiskForEvent(t *testing.T) {
	Setup()
	cache, _ := filepath.Abs(os.Getenv("SD_EVENT_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "event", local) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	defer os.RemoveAll("../data/cache/event/")
}


// test to copy pipeline cache files from shared storage pipeline directory
// to local build dir. Do NOT overwrite existing directories and files present
// in local build dir
func TestCache2DiskForPipelineToBuild(t *testing.T) {
	Setup()
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")
	os.RemoveAll("../data/cache/local/test")

	cachePath := filepath.Join(cache, local, "fromcache")
	os.MkdirAll(cachePath, 0777)

	testData := []byte("file from cache")
        err := ioutil.WriteFile(filepath.Join(cachePath, "test.txt"), testData, 0777)

	assert.Assert(t, Cache2Disk("get", "pipeline", local) == nil)

	_, err = os.Stat(filepath.Join(local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(local, "fromcache/test.txt"))
	assert.Assert(t, err == nil)

	defer os.RemoveAll("../data/cache/local/fromcache")
	defer os.RemoveAll("../data/cache/pipeline/")
}

