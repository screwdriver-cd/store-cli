package sdstore

import (
	"gotest.tools/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Init(cleanup bool) error {
	_ = os.Setenv("SD_PIPELINE_CACHE_DIR", "../data/cache/pipeline")
	_ = os.Setenv("SD_JOB_CACHE_DIR", "../data/cache/job")
	_ = os.Setenv("SD_EVENT_CACHE_DIR", "../data/cache/event")
	home, _ := os.UserHomeDir()

	if cleanup == true {
		os.RemoveAll(filepath.Join(home, "/tmp/storeclicache"))
		os.RemoveAll("../data/cache/local/fromcache")
		os.RemoveAll(os.Getenv("SD_PIPELINE_CACHE_DIR"))
		os.RemoveAll(os.Getenv("SD_EVENT_CACHE_DIR"))
		os.RemoveAll(os.Getenv("SD_JOB_CACHE_DIR"))

		_ = os.Setenv("SD_PIPELINE_CACHE_DIR", "")
		_ = os.Setenv("SD_JOB_CACHE_DIR", "")
		_ = os.Setenv("SD_EVENT_CACHE_DIR", "")
	} else {
		cacheDir, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
		_ = os.MkdirAll(cacheDir, 0777)
		cacheDir, _ = filepath.Abs(os.Getenv("SD_EVENT_CACHE_DIR"))
		_ = os.MkdirAll(cacheDir, 0777)
		cacheDir, _ = filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
		_ = os.MkdirAll(cacheDir, 0777)
		cacheDir = filepath.Join(home, "/tmp/storeclicache/server")
		_ = os.MkdirAll(cacheDir, 0777)
	}

	return nil
}

func TestCache2DiskInit(t *testing.T) {
	err := Init(false)
	assert.NilError(t, err, nil)
}

// test to validate invalid command
func TestCache2DiskInvalidCommand(t *testing.T) {
	err := Cache2Disk("", "pipeline", "", false, false, 0)
	assert.ErrorContains(t, err, "error: <nil>, command:  is not expected")
}

// test to validate invalid cache scope
func TestCache2DiskInvalidCacheScope(t *testing.T) {
	err := Cache2Disk("set", "   ", "", false, false, 0)
	assert.ErrorContains(t, err, "error: <nil>, cache scope  empty")
}

// test to validate invalid src and cache path for set
func TestCache2DiskInvalidSrcPathSet(t *testing.T) {
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local", false, false, 0)
	assert.ErrorContains(t, err, "no such file or directory")
}

// test to validate invalid src and cache path for get
func TestCache2DiskInvalidSrcPathGet(t *testing.T) {
	err := Cache2Disk("get", "pipeline", "../nodirectory/cache/local", false, false, 0)
	assert.Assert(t, err == nil)
}

// test to validate invalid src and cache path for remove
func TestCache2DiskInvalidSrcPathRemove(t *testing.T) {
	err := Cache2Disk("remove", "pipeline", "../nodirectory/cache/local", false, false, 0)
	assert.Assert(t, err == nil)
}

// test to copy cache files from local build dir to shared storage
// pipeline directory
func TestCache2DiskForPipeline(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "pipeline", local, false, true, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(cache, local, "local.txt"))
	assert.Assert(t, err == nil)
}

// test to copy cache files from local build dir to shared storage
// job directory

func TestCache2DiskForJobMaxSize(t *testing.T) {
	src, _ := filepath.Abs("../data/cache/2mb")
	err := Cache2Disk("set", "job", src, false, false, 1)
	assert.ErrorContains(t, err, " is more than allowed max limit 1MB")
}


// test to copy cache files from local build dir to shared storage
// job directory
func TestCache2DiskForJob(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "job", local, false, false, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(cache, local, "local.txt"))
	assert.Assert(t, err == nil)
}

// test to copy cache files from local build dir to shared storage
// job directory
func TestCache2DiskForJobWithCompress(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "job", local, true, false, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, filepath.Dir(local), "local.zip"))
	assert.Assert(t, err == nil)
}

// test to cache files from shared storage to local build dir
// job directory
func TestCache2DiskForJobGetWithCompress(t *testing.T) {
	// cache, _ := filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")
	_ = os.RemoveAll("../data/cache/local/test")

	assert.Assert(t, Cache2Disk("get", "job", local, true, false, 0) == nil)

	_, err := os.Stat(filepath.Join(local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(local, "local.txt"))
	assert.Assert(t, err == nil)
}

// test to cache files from shared storage to local build dir
// job directory
func TestCache2DiskForJobGetWithCompressNoFile(t *testing.T) {
	// cache, _ := filepath.Abs(os.Getenv("SD_JOB_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local1")

	assert.Assert(t, Cache2Disk("get", "job", local, true, false, 0) == nil)
}

// test to copy cache files from local build dir to shared storage
// event directory
func TestCache2DiskForEvent(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_EVENT_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "event", local, false, false, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(cache, local, "local.txt"))
	assert.Assert(t, err == nil)
}

// test to copy cache files from local build dir to shared storage
// pipeline directory
func TestCache2DiskForPipelineRerun(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("set", "pipeline", local, false, true, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(cache, local, "local.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(cache, filepath.Dir(local), "md5.json"))
	assert.Assert(t, err == nil)
}

// test to copy pipeline cache files from shared storage pipeline directory
// to local build dir. Do NOT overwrite existing directories and files present
// in local build dir
func TestCache2DiskForPipelineToBuild(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")
	_ = os.RemoveAll("../data/cache/local/test")

	cacheDir := filepath.Join(cache, local, "fromcache")
	_ = os.MkdirAll(cacheDir, 0777)

	testData := []byte("file from cache")
	err := ioutil.WriteFile(filepath.Join(cacheDir, "test.txt"), testData, 0777)

	assert.Assert(t, Cache2Disk("get", "pipeline", local, false, false, 0) == nil)

	_, err = os.Stat(filepath.Join(local, "test/test.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(local, "local.txt"))
	assert.Assert(t, err == nil)

	_, err = os.Stat(filepath.Join(local, "fromcache/test.txt"))
	assert.Assert(t, err == nil)
}

// test to remove cache files from shared storage pipeline directory
func TestCache2DiskRemoveCache(t *testing.T) {
	cache, _ := filepath.Abs(os.Getenv("SD_PIPELINE_CACHE_DIR"))
	local, _ := filepath.Abs("../data/cache/local")

	assert.Assert(t, Cache2Disk("remove", "pipeline", "../data/cache/local", false, false, 0) == nil)

	_, err := os.Stat(filepath.Join(cache, local))
	assert.ErrorContains(t, err, "no such file or directory")
}

// test to copy cache files from local build dir to shared storage
// pipeline directory
func TestCache2DiskForPipelineWithTilde(t *testing.T) {
	_ = os.Setenv("SD_PIPELINE_CACHE_DIR", "~/tmp/storeclicache/server")
	local := "~/tmp/storeclicache/local"
	home, _ := os.UserHomeDir()
	localDir := filepath.Join(home, "/tmp/storeclicache", "local")
	cache := filepath.Join(home, "/tmp/storeclicache", "server")

	_ = os.MkdirAll(localDir, 0777)
	testData := []byte("file from cache")
	err := ioutil.WriteFile(filepath.Join(localDir, "test.txt"), testData, 0777)

	assert.Assert(t, Cache2Disk("set", "pipeline", local, false, false, 0) == nil)

	_, err = os.Stat(filepath.Join(cache, localDir, "test.txt"))
	assert.Assert(t, err == nil)
}

// test to validate invalid cache scope
func TestCache2DiskInvalidCacheDirectory(t *testing.T) {
	cacheDir, _ := filepath.Abs("../data/cache/nodirectory/pipeline")
	_ = os.Setenv("SD_PIPELINE_CACHE_DIR", cacheDir)
	err := Cache2Disk("set", "pipeline", "", false, false, 0)
	assert.ErrorContains(t, err, "no such file or directory")
}

func TestCleanup(t *testing.T) {
	err := Init(true)
	assert.NilError(t, err, nil)
}
