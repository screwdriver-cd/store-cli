# store-cli

CLI for communicating with the Screwdriver store.

```
$ go run store-cli.go 
NAME:
   store - get, set or remove items in the Screwdriver store

USAGE:
   [options]

VERSION:
   0.0.0

COMMANDS:
     get      Get a new item from the store
     set      Put a new item to the store
     remove   Remove an existing item from the store
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --scope value  Scope of command. For example: event, build, pipeline
   --type value   Type of the command. For example: cache, artifacts, steps (default: "stable")
   --help, -h     show help
   --version, -v  print the version

COPYRIGHT:
   (c) 2018 Yahoo Inc.
```

## Build cache

To use the `store-cli` tool for caching files and folders in your Screwdriver builds, you can specify `--type=cache` and the `scope` of your cache.

| Scope  | Access |
|---|---|
| pipeline  | Cache accessible to all builds in the same pipeline  |
| event  | Cache accessible to all builds in the same event  |
| job  | Cache accessible to all builds for the same job  |

For example, if you want to cache the `node_modules` folder within the `event` scope, simply run `store-cli set node_modules/ --scope=event --type=cache` and `store-cli get node_modules/ --scope=event --type=cache` (to restore the cache).
