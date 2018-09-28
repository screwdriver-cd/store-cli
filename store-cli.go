package main

import (
	"fmt"
	"net/url"
	"os"
	"runtime/debug"

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
	version := "v1"

	var scopeEnv string
	switch scope {
	case "events":
		scopeEnv = os.Getenv("SD_EVENT_ID")
	case "jobs":
		scopeEnv = os.Getenv("SD_JOB_ID")
	case "pipelines":
		scopeEnv = os.Getenv("SD_PIPELINE_ID")
	}

	var path string
	switch storeType {
	case "cache":
		path = "/caches/" + scope + "/" + scopeEnv + "/" + key
	case "artifacts":
		path = "/builds/" + os.Getenv("SD_BUILD_ID") + "-ARTIFACTS/" + key
	case "logs":
		path = "/builds/" + os.Getenv("SD_BUILD_ID") + "-" + key
	default:
		path = ""
	}

	if len(path) == 0 {
		return nil, fmt.Errorf("Invalid parameters")
	}

	var fullpath *url.URL
	fullpath, err := url.Parse(storeURL + "/" + version)
	if err != nil {
		return nil, fmt.Errorf("Error parsing store url")
	}
	fullpath.Path += path

	return fullpath, nil
}

func get(storeType, scope, key string) error {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return err
	}
	store := sdstore.NewStore(sdToken)
	_, err = store.Download(fullURL)

	return err
}

func set(storeType, scope, key, filePath string) error {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return err
	}
	store := sdstore.NewStore(sdToken)
	return store.Upload(fullURL, filePath)
}

func remove(storeType, scope, key string) error {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
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
				if len(c.Args()) == 0 {
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
				if len(c.Args()) <= 1 {
					return cli.ShowAppHelp(c)
				}
				scope := c.String("scope")
				storeType := c.String("type")
				key := c.Args().Get(0)
				val := c.Args().Get(1)
				err := set(storeType, scope, key, val)
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
				if len(c.Args()) <= 1 {
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
