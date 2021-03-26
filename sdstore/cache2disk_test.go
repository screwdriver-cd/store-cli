package sdstore

import (
	"fmt"
	copy2 "github.com/otiai10/copy"
	"gotest.tools/assert"
	"io/ioutil"
	// "github.com/gofrs/flock"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const CompressFormat = ".tar.zst"

func init() {
	removeCacheFolders()

	wd, _ := os.Getwd()
	cacheTestFolder1 := filepath.Join(wd, "../data/cache/.m2/testfolder1")
	cacheTestFolder2 := filepath.Join(wd, "../data/cache/.m2/testfolder2")
	cacheTestFolder2Lib := filepath.Join(wd, "../data/cache/.m2/testfolder2/lib")
	cacheTestFolder2Utils := filepath.Join(wd, "../data/cache/.m2/testfolder2/utils")
	cacheMaxSize := filepath.Join(wd, "../data/cache/maxsize")

	_ = os.MkdirAll(cacheTestFolder1, 0777)
	_ = os.MkdirAll(cacheTestFolder2Lib, 0777)
	_ = os.MkdirAll(cacheTestFolder2Utils, 0777)
	_ = os.MkdirAll(cacheMaxSize, 0777)

	data := []byte("file from cache")
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder1, "testfolder1.txt"), data, 0777)
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder1, "testfolder1.2.txt"), data, 0777)
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder1, "testfolder1.3.txt"), data, 0777)
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder2, "testfolder2.txt"), data, 0777)
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder2Lib, "lib.txt"), data, 0777)
	_ = ioutil.WriteFile(filepath.Join(cacheTestFolder2Utils, "utils.txt"), data, 0777)

	data2mb := make([]byte, 2097152)
	_ = ioutil.WriteFile(filepath.Join(cacheMaxSize, "2mb"), data2mb, 0777)
}

func removeCacheFolders() {
	wd, _ := os.Getwd()
	dir := filepath.Join(wd, "../data/cache")
	_ = os.RemoveAll(dir)

	home, _ := os.UserHomeDir()
	dir = filepath.Join(home, "tmp/storecli/")
	_ = os.RemoveAll(dir)
}

// test to validate invalid command
func TestCache2DiskInvalidCommand(t *testing.T) {
	err := Cache2Disk("", "pipeline", "", 0)
	assert.ErrorContains(t, err, "command:  is not expected")
}

// test to validate invalid cache scope
func TestCache2DiskInvalidCacheScope(t *testing.T) {
	err := Cache2Disk("set", "   ", "", 0)
	assert.ErrorContains(t, err, "cache scope  empty")
}

// test to validate invalid src and cache path for set
func TestCache2DiskInvalidSrcPathSet(t *testing.T) {
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local", 0)
	assert.ErrorContains(t, err, "set cache FAILED")
}

// test to validate invalid src and cache path for get
func TestCache2DiskInvalidSrcPathGet(t *testing.T) {
	err := Cache2Disk("get", "pipeline", "../nodirectory/cache/local", 0)
	assert.Assert(t, err == nil)
}

// test to validate invalid src and cache path for remove
func TestCache2DiskInvalidSrcPathRemove(t *testing.T) {
	err := Cache2Disk("remove", "pipeline", "../nodirectory/cache/local", 0)
	assert.Assert(t, err == nil)
}

// test to validate invalid src and cache path for set
func TestCache2DiskInvalidBaseCacheDir(t *testing.T) {
	_ = os.Setenv("SD_PIPELINE_CACHE_DIR", "../nodirectory/cache/local")
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local", 0)
	assert.ErrorContains(t, err, "not found")
}

func Test_SetCache_wCompress_File_CherryPick(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/.m2/testfolder1/testfolder1.2.txt", "../data/cache/.m2/testfolder1/testfolder1.3.txt"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_File(t *testing.T) {
	var err error

	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_RewriteFile_NODELTA(t *testing.T) {
	var info os.FileInfo
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	time.Sleep(2 * time.Second) // pause test for 2 seconds
	currentTime := time.Now().Unix()

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)
			info, _ = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, info.ModTime().Unix() < currentTime)
		}
	}
}

func Test_GetCache_wCompress_File(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			_ = os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, 0) == nil)

			_, err := os.Lstat(local)
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_NewFolder_CherryPick(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2/lib", "../data/cache/.m2/testfolder2/utils"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_GetCache_wCompress_Folder_CherryPick(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2/lib", "../data/cache/.m2/testfolder2/utils"}

	currentTime := time.Now().Unix()
	fmt.Println(currentTime)

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			_ = os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, 0) == nil)

			_, err := os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_wCompress_NewFolder(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_RewriteFolder_NODELTA(t *testing.T) {
	var info os.FileInfo
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	time.Sleep(2 * time.Second) // pause test for 2 seconds
	currentTime := time.Now().Unix()
	fmt.Println(time.Now().Format(http.TimeFormat))

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			assert.Assert(t, Cache2Disk("set", cache[0], local, 0) == nil)

			info, _ = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, info.ModTime().Unix() < currentTime)
		}
	}
}

func Test_GetCache_wCompress_Folder(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	currentTime := time.Now().Unix()
	fmt.Println(currentTime)

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			_ = os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, 0) == nil)

			_, err := os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_GetCache_wCompress_Folder_doNOTOverwriteNewFilesInLocal(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	currentTime := time.Now().Unix()
	fmt.Println(currentTime)

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)
			testData := []byte("created to test donotoverwrite local file scenario")
			err := ioutil.WriteFile(filepath.Join(local, "donotoverwrite.txt"), testData, 0777)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, 0) == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", "donotoverwrite", ".txt")))
			assert.Assert(t, err == nil)

			_ = os.RemoveAll(filepath.Join(local, "donotoverwrite.txt"))
		}
	}
}

func Test_RemoveCache_Folder_wCompress(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("remove", cache[0], local, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.ErrorContains(t, err, "no such file or directory")
			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_NewFolder_wCompress_wTilde(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1"}
	tildeCacheFolders := []string{"~/tmp/storecli/cache"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for index, eachFolder := range tildeCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			local := filepath.Join(home, strings.ReplaceAll(eachFolder, "~", ""))
			actualLocal, _ := filepath.Abs(localCacheFolders[index])
			_ = copy2.Copy(actualLocal, local)
			fmt.Printf("local tilde folder is [%s]", local)

			// compress: false
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_MaxSizeError(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: false
			err := Cache2Disk("set", cache[0], local, 1)
			assert.ErrorContains(t, err, "set cache FAILED")
		}
	}
}

func Test_SetCache_NewRelativeFolder_wCompress(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")
			_ = os.Chdir(dir)
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
	_ = os.Chdir(origDir)
}

func Test_GetCache_NewRelativeFolder_wCompress(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		fmt.Println(cache[2])
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], ss)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")
			_ = os.Chdir(dir)
			_ = os.RemoveAll(cache[0])
			assert.Assert(t, Cache2Disk("get", cache[0], eachFolder, 0) == nil)
			_, err := os.Lstat(filepath.Join(dir, eachFolder))
			assert.Assert(t, err == nil)
		}
	}
	_ = os.Chdir(origDir)
}

func Test_SetCache_Lock_NewRelativeFolder_wCompress(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		FlockWaitMinSecs = 1
		FlockWaitMaxSecs = 2

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")

			go func() {
				_ = os.MkdirAll(filepath.Join(cacheDir, eachFolder), os.ModePerm)
				_, err := os.OpenFile(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s%s", filepath.Base(eachFolder), CompressFormat, ".lock")), os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.ModePerm)
				time.Sleep(10 * time.Second)
				if err == nil {
					defer func() {
						_ = os.Remove(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s%s", filepath.Base(eachFolder), CompressFormat, ".lock")))
					}()
				}
			}()
			time.Sleep(2 * time.Second)
			_ = os.Chdir(dir)
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, 0) == nil)
			_, err := os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
	_ = os.Chdir(origDir)
}

func Test_SetCache_Lock_Fail_NewRelativeFolder_wCompress(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	FlockWaitMinSecs = 1
	FlockWaitMaxSecs = 2

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")

			go func() {
				_ = os.MkdirAll(filepath.Join(cacheDir, eachFolder), os.ModePerm)
				_, err := os.OpenFile(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s%s", filepath.Base(eachFolder), CompressFormat, ".lock")), os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.ModePerm)
				time.Sleep(20 * time.Second)
				if err == nil {
					defer func() {
						_ = os.Remove(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s%s", filepath.Base(eachFolder), CompressFormat, ".lock")))
					}()
				}
			}()
			time.Sleep(2 * time.Second)
			_ = os.Chdir(dir)
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, 0) != nil)
		}
	}
	_ = os.Chdir(origDir)
}


func Test_BackwardCompatibility_Zip_Folder(t *testing.T) {
	localFolder, _ := filepath.Abs("../data/cache/.m2/testfolder1")
	cacheFolder, _ := filepath.Abs("../data/cache/pipeline")
	cacheFolder = filepath.Join(cacheFolder, localFolder)
	_ = os.RemoveAll(cacheFolder)
	_ = os.MkdirAll(cacheFolder, 0777)
	cacheFile := fmt.Sprintf("%s/%s", cacheFolder, "testfolder1.zip")
	_ = Zip(localFolder, cacheFile)

	_ = os.RemoveAll(localFolder)
	assert.Assert(t, Cache2Disk("get", "pipeline", localFolder, 0) == nil)
	_, err := os.Lstat(filepath.Join(localFolder, fmt.Sprintf("%s%s", filepath.Base(localFolder), ".txt")))
	assert.Assert(t, err == nil)
}

func Test_BackwardCompatibility_Zip_File(t *testing.T) {
	localFolder, _ := filepath.Abs("../data/cache/.m2/testfolder1/testfolder1.txt")
	cacheFolder, _ := filepath.Abs("../data/cache/pipeline")
	cacheFolder = filepath.Join(cacheFolder, filepath.Dir(localFolder))
	_ = os.RemoveAll(cacheFolder)
	_ = os.MkdirAll(cacheFolder, 0777)
	cacheFile := fmt.Sprintf("%s/%s", cacheFolder, "testfolder1.txt.zip")
	_ = Zip(localFolder, cacheFile)

	_ = os.RemoveAll(filepath.Dir(localFolder))
	assert.Assert(t, Cache2Disk("get", "pipeline", localFolder, 0) == nil)
	_, err := os.Lstat(filepath.Join(filepath.Dir(localFolder), filepath.Base(localFolder)))
	assert.Assert(t, err == nil)

	// Test removing zip file as part of set
	assert.Assert(t, Cache2Disk("set", "pipeline", localFolder, 0) == nil)
	_, err = os.Lstat(filepath.Join(cacheFolder, "testfolder1.txt.zip"))
	assert.Assert(t, err != nil)
	_, err = os.Lstat(filepath.Join(cacheFolder, "testfolder1.txt.tar.zst"))
	assert.Assert(t, err == nil)
}

func Test_SetCache_NewRelativeFolder_wCompress_GoLib(t *testing.T) {
	var err error
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		_ = os.RemoveAll(cacheDir)
		_ = os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")
			_ = os.Chdir(dir)
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, 0) == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), CompressFormat)))
			assert.Assert(t, err == nil)
			_, err = os.Lstat(filepath.Join(cacheDir, eachFolder, fmt.Sprintf("%s%s", filepath.Base(eachFolder), Md5Extension)))
			assert.Assert(t, err == nil)
		}
	}
	_ = os.Chdir(origDir)
}

func Test_GetCache_NewRelativeFolder_wCompress_GoLib(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline"}
	localCacheFolders := []string{"storecli"}

	origDir, _ := os.Getwd()
	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		fmt.Println(cache[2])
		ss, _ := filepath.Abs(cache[2])
		_ = os.Setenv(cache[1], ss)
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], ss)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, "tmp")
			_ = os.Chdir(dir)
			_ = os.RemoveAll(cache[0])
			assert.Assert(t, Cache2Disk("get", cache[0], eachFolder, 0) == nil)
			_, err := os.Lstat(filepath.Join(dir, eachFolder))
			assert.Assert(t, err == nil)
		}
	}
	_ = os.Chdir(origDir)
}

func Test_RemoveCache_Folders(t *testing.T) {
	removeCacheFolders()
}
