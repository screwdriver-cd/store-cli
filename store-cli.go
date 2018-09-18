package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"time"

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

	// artifacts and logs are in builds folder
	path := "builds/"
	if storeType == "cache" {
		path = "caches/"
	}

	switch scope {
	case "events":
		path += "events/" + os.Getenv("SD_EVENT_ID") + "/" + key
	case "builds":
		path += os.Getenv("SD_BUILD_ID") + "-" + key
	}

	fullpath := fmt.Sprintf("%s/%s/%s", storeURL, version, path)

	return url.Parse(fullpath)
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// handleResponse attempts to parse error objects from Screwdriver
func handleResponse(res *http.Response) ([]byte, error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d returned: %s", res.StatusCode, body)
	}
	return body, nil
}

func get(storeType, scope, key string, output io.Writer) error {
	sdToken := os.Getenv("SD_TOKEN")
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", fullURL.String(), strings.NewReader(""))
	if err != nil {
		return fmt.Errorf("Failed to create request about command to Store API: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sdToken))
	var client = &http.Client{
		Timeout: time.Second * 10,
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to get command from Store API: %v", err)
	}
	defer res.Body.Close()
	body, err := handleResponse(res)
	if err != nil {
		return err
	}
	return nil
}

func set(storeType, scope, key, val string) ([]byte, error) {
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return nil, err
	}

	sdToken := os.Getenv("SD_TOKEN")
	req, reqErr := http.NewRequest("PUT", fullURL.String(), strings.NewReader(""))
	if reqErr != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(sdToken))
	// req.Header.Set("Content-Type", bodyType)
	// req.ContentLength = size
	var client = &http.Client{
		Timeout: time.Second * 10,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
}

func remove(storeType, scope, key, val string) ([]byte, error) {
	fullURL, err := makeURL(storeType, scope, key)
	if err != nil {
		return nil, err
	}

	sdToken := os.Getenv("SD_TOKEN")
	req, reqErr := http.NewRequest("DELETE", fullURL.String(), strings.NewReader(""))
	if reqErr != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(sdToken))
	// req.Header.Set("Content-Type", bodyType)
	// req.ContentLength = size
	var client = &http.Client{
		Timeout: time.Second * 10,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
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
