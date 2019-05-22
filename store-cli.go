package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/screwdriver-cd/store-cli/sdstore"
	"github.com/urfave/cli"
)

// VERSION gets set by the build script via the LDFLAGS
var VERSION string

// successExit exits process with 0
func successExit() {
	os.Exit(0)
}

// failureExit exits process with 1
func failureExit(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	}
	os.Exit(1)
}

// finalRecover makes one last attempt to recover from a panic.
// This should only happen if the previous recovery caused a panic.
func finalRecover() {
	if p := recover(); p != nil {
		fmt.Fprintln(os.Stderr, "ERROR: Something terrible has happened. Please file a ticket with this info:")
		fmt.Fprintf(os.Stderr, "ERROR: %v\n%v\n", p, debug.Stack())
		failureExit(nil)
	}
	successExit()
}

// Skip cache action for PR jobs (event, pipeline scope)
func skipCache(storeType, scope, action string) bool {
	// if is not cache, or if job is not PR
	if storeType != "cache" || os.Getenv("SD_PULL_REQUEST") == "" {
		return false
	}

	// For PR jobs,
	// skip event cache to save time, since PR event only consists of 1 job
	// skip pipeline scoped unless it's trying to get
	// skip job scoped unless it's trying to get
	if scope == "event" || (action != "get" && (scope == "pipeline" || scope == "job")) {
		log.Printf("Skipping %s %s-scoped cache for Pull Request", action, scope)
		return true
	}

	return false
}

// makeURL creates the fully-qualified url for a given Store path
func makeURL(storeType, scope, key string) (*url.URL, error) {
	storeURL := os.Getenv("SD_STORE_URL")
	var scopeEnv string
	switch scope {
	case "event":
		scopeEnv = os.Getenv("SD_EVENT_ID")
	case "job":
		// use real job id if current job is a PR
		if os.Getenv("SD_PULL_REQUEST") != "" && os.Getenv("SD_PR_PARENT_JOB_ID") != "" {
			scopeEnv = os.Getenv("SD_PR_PARENT_JOB_ID")
		} else {
			scopeEnv = os.Getenv("SD_JOB_ID")
		}
	case "pipeline":
		scopeEnv = os.Getenv("SD_PIPELINE_ID")
	}

	var path string
	switch storeType {
	case "cache":
		// if path is relative, get abs path
		if strings.HasPrefix(key, "/") == false {
			key, _ = filepath.Abs(key)
		}

		key = strings.TrimRight(key, "/")
		encoded := url.PathEscape(key)
		path = "caches/" + scope + "s/" + scopeEnv + "/" + encoded
	case "artifact":
		key = strings.TrimPrefix(key, "./")
		encoded := url.PathEscape(key)
		path = "builds/" + os.Getenv("SD_BUILD_ID") + "/ARTIFACTS/" + encoded
	case "log":
		path = "builds/" + os.Getenv("SD_BUILD_ID") + "-" + key
	default:
		path = ""
	}

	if len(path) == 0 {
		return nil, fmt.Errorf("Invalid parameters")
	}

	fullpath := fmt.Sprintf("%s%s", storeURL, path)

	return url.Parse(fullpath)
}

func get(storeType, scope, key string) error {
	if skipCache(storeType, scope, "get") {
		return nil
	}

	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)

	if err != nil {
		return err
	}
	store := sdstore.NewStore(sdToken)

	var toExtract bool

	if storeType == "cache" {
		toExtract = true
	} else {
		toExtract = false
	}

	_, err = store.Download(fullURL, toExtract)

	return err
}

func set(storeType, scope, filePath string) error {
	if skipCache(storeType, scope, "set") {
		return nil
	}
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, filePath)

	if err != nil {
		return err
	}
	store := sdstore.NewStore(sdToken)

	var toCompress bool

	if storeType == "cache" {
		toCompress = true
	} else {
		toCompress = false
	}

	return store.Upload(fullURL, filePath, toCompress)
}

func remove(storeType, scope, key string) error {
	if skipCache(storeType, scope, "remove") {
		return nil
	}

	sdToken := os.Getenv("SD_TOKEN")
	store := sdstore.NewStore(sdToken)

	if storeType == "cache" {
		md5URL, err := makeURL(storeType, scope, fmt.Sprintf("%s%s", filepath.Clean(key), "_md5.json"))
		if err != nil {
			return err
		}

		err = store.Remove(md5URL)
		if err != nil {
			return fmt.Errorf("Failed to remove file from %s: %s", md5URL.String(), err)
		}

		zipURL, err := makeURL(storeType, scope, fmt.Sprintf("%s%s", filepath.Clean(key), ".zip"))
		if err != nil {
			return err
		}

		err = store.Remove(zipURL)
		if err != nil {
			return fmt.Errorf("Failed to remove file from %s: %s", zipURL.String(), err)
		}

		return nil
	}

	fullURL, err := makeURL(storeType, scope, key)

	if err != nil {
		return err
	}
	return store.Remove(fullURL)
}

func main() {
	defer finalRecover()

	app := cli.NewApp()
	app.Name = "store"
	app.Usage = "CLI to communicate with Screwdriver Store"
	app.UsageText = "[options]"
	app.Copyright = "(c) 2018 Yahoo Inc."
	app.Usage = "get, set or remove items in the Screwdriver store"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "scope",
			Usage: "Scope of command. For example: event, build, pipeline",
			Value: "",
		},
		cli.StringFlag{
			Name:  "type",
			Usage: "Type of the command. For example: cache, artifacts, steps",
			Value: "stable",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "get",
			Usage: "Get a new item from the store",
			Action: func(c *cli.Context) error {
				if len(c.Args()) != 1 {
					return cli.ShowAppHelp(c)
				}
				scope := c.String("scope")
				storeType := c.String("type")

				key := c.Args().Get(0)
				err := get(storeType, scope, key)
				if err != nil {
					failureExit(err)
				}
				successExit()
				return nil
			},
			Flags: app.Flags,
		},
		{
			Name:  "set",
			Usage: "Put a new item to the store",
			Action: func(c *cli.Context) error {
				if len(c.Args()) != 1 {
					return cli.ShowAppHelp(c)
				}
				scope := c.String("scope")
				storeType := c.String("type")
				key := c.Args().Get(0)
				err := set(storeType, scope, key)
				if err != nil {
					failureExit(err)
				}
				successExit()
				return nil
			},
			Flags: app.Flags,
		},
		{
			Name:  "remove",
			Usage: "Remove an existing item from the store",
			Action: func(c *cli.Context) error {
				if len(c.Args()) != 1 {
					return cli.ShowAppHelp(c)
				}
				scope := c.String("scope")
				storeType := c.String("type")
				key := c.Args().Get(0)
				err := remove(storeType, scope, key)
				if err != nil {
					failureExit(err)
				}
				successExit()
				return nil
			},
			Flags: app.Flags,
		},
	}

	app.Run(os.Args)
}
