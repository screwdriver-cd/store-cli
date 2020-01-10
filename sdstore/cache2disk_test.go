package sdstore

import (
	"fmt"
	copy2 "github.com/otiai10/copy"
	"gotest.tools/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)


// test to validate invalid command
func TestCache2DiskInvalidCommand(t *testing.T) {
	err := Cache2Disk("", "pipeline", "", false, false, 0)
	assert.ErrorContains(t, err, "command:  is not expected")
}

// test to validate invalid cache scope
func TestCache2DiskInvalidCacheScope(t *testing.T) {
	err := Cache2Disk("set", "   ", "", false, false, 0)
	assert.ErrorContains(t, err, "cache scope  empty")
}

// test to validate invalid src and cache path for set
func TestCache2DiskInvalidSrcPathSet(t *testing.T) {
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local", false, false, 0)
	assert.ErrorContains(t, err, "set cache FAILED")
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

// test to validate invalid src and cache path for set
func TestCache2DiskInvalidBaseCacheDir(t *testing.T) {
	_ = os.Setenv("SD_PIPELINE_CACHE_DIR", "../nodirectory/cache/local")
	err := Cache2Disk("set", "pipeline", "../nodirectory/cache/local", false, false, 0)
	assert.ErrorContains(t, err, "not found")
}

func Test_SetCache_wCompress_File_CherryPick(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/.m2/testfolder1/testfolder1.2.txt", "../data/cache/.m2/testfolder1/testfolder1.3.txt"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_File(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_RewriteFile_NODELTA(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	time.Sleep(2 * time.Second)	// pause test for 2 seconds
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

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			info, _ := os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)

			info, _ = os.Lstat(filepath.Join(cacheDir, filepath.Dir(local), fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)
		}
	}
}

func Test_GetCache_wCompress_File(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

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
			os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(local)
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_File_CherryPick(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/.m2/testfolder1/testfolder1.2.txt", "../data/cache/.m2/testfolder1/testfolder1.3.txt"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, fmt.Sprintf("%s%s", local, ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_File(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, fmt.Sprintf("%s%s", local, ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_RewriteFile_NODELTA(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	time.Sleep(2 * time.Second)	// pause test for 2 seconds
	currentTime := time.Now().Unix()
	fmt.Println(currentTime)

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, true, 0) == nil)

			info, _ := os.Lstat(filepath.Join(cacheDir, local))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)

			info, _ = os.Lstat(filepath.Join(cacheDir, fmt.Sprintf("%s%s", local, ".md5")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)
		}
	}
}

func Test_SetCache_File_NoMD5Check(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, false, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, fmt.Sprintf("%s%s", local, ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_GetCache_File(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1/testfolder1.txt", "../data/cache/maxsize/2mb"}

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
			os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(local)
			assert.Assert(t, err == nil)
		}
	}
}

func Test_RemoveCache_File(t *testing.T) {
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

			// compress: false
			assert.Assert(t, Cache2Disk("remove", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_wCompress_NewFolder_CherryPick(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2/lib", "../data/cache/.m2/testfolder2/utils"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
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
			os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_wCompress_NewFolder(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: true
			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_wCompress_RewriteFolder_NODELTA(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	time.Sleep(2 * time.Second)	// pause test for 2 seconds
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

			assert.Assert(t, Cache2Disk("set", cache[0], local, true, true, 0) == nil)

			info, _ := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)

			info, _ = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)
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
			os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
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
			assert.Assert(t, Cache2Disk("get", cache[0], local, true, true, 0) == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", "donotoverwrite", ".txt")))
			assert.Assert(t, err == nil)

			os.RemoveAll(filepath.Join(local, "donotoverwrite.txt"))
		}
	}
}

func Test_RemoveCache_Folder_wCompress(t *testing.T) {
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
			assert.Assert(t, Cache2Disk("remove", cache[0], local, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_NewFolder(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: false
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

// test to copy cache files from local build dir to shared storage
// pipeline directory
func Test_SetCache_RewriteFolder_NODELTA(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	time.Sleep(2 * time.Second)	// pause test for 2 seconds
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

			assert.Assert(t, Cache2Disk("set", cache[0], local, false, true, 0) == nil)

			info, _ := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)

			info, _ = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			fmt.Printf("currentTime: [%v], file: [%v], createTime: [%v]\n", currentTime, fmt.Sprintf("%s%s", filepath.Base(local), ".zip"), int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec))
			assert.Assert(t, int64(info.Sys().(*syscall.Stat_t).Ctimespec.Sec) < currentTime)
		}
	}
}

func Test_SetCache_NewFolder_NoMD5Check(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1", "../data/cache/.m2/testfolder2"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: false
			assert.Assert(t, Cache2Disk("set", cache[0], local, false, false, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_GetCache_Folder(t *testing.T) {
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
			os.RemoveAll(local)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_GetCache_Folder_doNOTOverwriteNewFilesInLocal(t *testing.T) {
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
			testData := []byte("created to test donotoverwrite local file scenario")
			err := ioutil.WriteFile(filepath.Join(local, "donotoverwrite.txt"), testData, 0777)

			// compress: true
			assert.Assert(t, Cache2Disk("get", cache[0], local, false, true, 0) == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(local, fmt.Sprintf("%s%s", "donotoverwrite", ".txt")))
			assert.Assert(t, err == nil)

			os.RemoveAll(filepath.Join(local, "donotoverwrite.txt"))
		}
	}
}

func Test_RemoveCache_Folder(t *testing.T) {
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

			// compress: false
			assert.Assert(t, Cache2Disk("remove", cache[0], local, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".txt")))
			assert.ErrorContains(t, err, "no such file or directory")

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.ErrorContains(t, err, "no such file or directory")
		}
	}
}

func Test_SetCache_NewFolder_wCompress_wTilde(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1"}
	tildeCacheFolders := []string{"~/tmp/storecli/cache"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for index, eachFolder := range tildeCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			local := filepath.Join(home, strings.ReplaceAll(eachFolder, "~", ""))
			actualLocal, _ := filepath.Abs(localCacheFolders[index])
			copy2.Copy(actualLocal, local)
			fmt.Printf("local tilde folder is [%s]", local)

			// compress: false
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, true, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".zip")))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
			assert.Assert(t, err == nil)
		}
	}
}

func Test_SetCache_NewFolder_wTilde(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/.m2/testfolder1"}
	tildeCacheFolders := []string{"~/tmp/storecli/cache"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for index, eachFolder := range tildeCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			home, _ := os.UserHomeDir()
			local := filepath.Join(home, strings.ReplaceAll(eachFolder, "~", ""))
			actualLocal, _ := filepath.Abs(localCacheFolders[index])
			copy2.Copy(actualLocal, local)
			fmt.Printf("local tilde folder is [%s]", local)

			// compress: false
			assert.Assert(t, Cache2Disk("set", cache[0], eachFolder, false, true, 0) == nil)

			_, err := os.Lstat(filepath.Join(cacheDir, local, "testfolder1.txt"))
			assert.Assert(t, err == nil)

			_, err = os.Lstat(filepath.Join(cacheDir, local, fmt.Sprintf("%s%s", filepath.Base(local), ".md5")))
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
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: false
			err := Cache2Disk("set", cache[0], local, true, true, 1)
			assert.ErrorContains(t, err, "set cache FAILED")
		}
	}
}

func Test_SetCache_MaxSizeError(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}
	localCacheFolders := []string{"../data/cache/maxsize/2mb"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		_ = os.Setenv(cache[1], cache[2])
		cacheDir, _ := filepath.Abs(os.Getenv(cache[1]))
		fmt.Printf("cache scope is [%v], cache environment variable is [%v], cache directory is [%v]\n", cache[0], cache[1], cacheDir)
		os.RemoveAll(cacheDir)
		os.MkdirAll(cacheDir, 0777)

		for _, eachFolder := range localCacheFolders {
			fmt.Printf("local cache folder is [%s]\n", eachFolder)
			local, _ := filepath.Abs(eachFolder)

			// compress: false
			err := Cache2Disk("set", cache[0], local, false, true, 1)
			assert.ErrorContains(t, err, "set cache FAILED")
		}
	}
}

func Test_RemoveCache_Folders(t *testing.T) {
	cacheScope := []string{"pipeline:SD_PIPELINE_CACHE_DIR:../data/cache/pipeline", "job:SD_JOB_CACHE_DIR:../data/cache/job", "event:SD_EVENT_CACHE_DIR:../data/cache/event"}

	for _, eachCacheScope := range cacheScope {
		cache := strings.Split(eachCacheScope, ":")
		cacheDir, _ := filepath.Abs(cache[2])
		defer os.RemoveAll(cacheDir)
	}
}