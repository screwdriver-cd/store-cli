package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/urfave/cli"
)

// VERSION gets set by the build script via the LDFLAGS
var VERSION string

// habDepotURL is base url for public depot of habitat
const habDepotURL = "https://willem.habitat.sh/v1/depot"

var habPath = "/opt/sd/bin/hab"
var versionValidator = regexp.MustCompile(`^\d+(\.\d+)*$`)
var execCommand = exec.Command

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

func main() {
	defer finalRecover()
	var err error

	app := cli.NewApp()
	app.Name = "store"
	app.Usage = "CLI to communicate with Screwdriver Store"
	app.UsageText = "sd-step command arguments [options]"
	app.Copyright = "(c) 2018 Yahoo Inc."
  app.Usage = "get or set metadata for Screwdriver build"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "scope",
			Usage:       "Scope of command. For example: event, build, pipeline",
			Value:       "",
		},
		cli.StringFlag{
			Name:        "type",
			Usage:       "Type of the command. For example: cache, artifacts, steps",
			Value:       "stable",
		},
	}

  app.Commands = []cli.Command{
  		{
  			Name:  "get",
  			Usage: "Put a new item to the store",
  			Action: func(c *cli.Context) error {
  				if len(c.Args()) == 0 {
  					return cli.ShowAppHelp(c)
  				}
  				key := c.Args().Get(0)
  				err := get(key, os.Stdout)
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
  				key := c.Args().Get(0)
  				val := c.Args().Get(1)
  				err := set(key, val)
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
