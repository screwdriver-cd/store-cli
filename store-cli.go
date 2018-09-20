package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime/debug"

	"github.com/urfave/cli"
	"github.com/screwdriver-cd/store-cli/sdstore"
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

	var folder string
	switch storeType {
	case "cache":
		folder = "caches"
	case "logs":
	case "artifacts":
		folder = "builds"
	}

	var path string
	switch scope {
	case "events":
		path = "events/" + os.Getenv("SD_EVENT_ID") + "/" + key
	case "builds":
		path = os.Getenv("SD_BUILD_ID") + "-" + key
	}

	fullpath := fmt.Sprintf("%s/%s/%s/%s", storeURL, version, folder, path)

	return url.Parse(fullpath)
}

func get(storeType, scope, key string, output io.Writer) error {
	storeURL := os.Getenv("SD_STORE_URL")
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return err
	}
	store := NewStore(storeURL, sdToken);
	return store.Download(fullURL)
}

func set(storeType, scope, key, val string) ([]byte, error) {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func remove(storeType, scope, key, val string) ([]byte, error) {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func main() {
	defer finalRecover()
	var err error

	app := cli.NewApp()
	app.Name = "store"
	app.Usage = "CLI to communicate with Screwdriver Store"
	app.UsageText = "[options]"
	app.Copyright = "(c) 2018 Yahoo Inc."
	app.Usage = "get, set or delete items in the Screwdriver store"

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
				err := get(storeType, scope, key, os.Stdout)
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
				response, err := set(storeType, scope, key, val)
				if err != nil {
					failureExit(err)
				}
				successExit()
				return nil
			},
			Flags: app.Flags,
		},
		{
			Name:  "delete",
			Usage: "Delete an existing item from the store",
			Action: func(c *cli.Context) error {
				if len(c.Args()) <= 1 {
					return cli.ShowAppHelp(c)
				}
				scope := c.String("scope")
				storeType := c.String("type")
				key := c.Args().Get(0)
				val := c.Args().Get(1)
				response, err := remove(storeType, scope, key, val)
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
