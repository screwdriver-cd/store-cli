package main

import (
	"fmt"
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

// makeURL creates the fully-qualified url for a given Store path
func makeURL(storeType, scope, key string) (*url.URL, error) {
	storeURL := os.Getenv("SD_STORE_URL")

	var scopeEnv string
	switch scope {
	case "event":
		scopeEnv = os.Getenv("SD_EVENT_ID")
	case "job":
		scopeEnv = os.Getenv("SD_JOB_ID")
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
		encoded := url.QueryEscape(key)
		path = "caches/" + scope + "s/" + scopeEnv + "/" + encoded
	case "artifact":
		path = "builds/" + os.Getenv("SD_BUILD_ID") + "-ARTIFACTS/" + key
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
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)

	fmt.Printf("=======URL IS %v\n", fullURL)
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
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, filePath)
	fmt.Printf("=======URL IS %v\n", fullURL)
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
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	fmt.Printf("=======URL IS %v\n", fullURL)
	if err != nil {
		return err
	}
	store := sdstore.NewStore(sdToken)
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
				if len(c.Args()) > 1 {
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
