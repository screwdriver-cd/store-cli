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
   --upload-timeout value    Specifies the upload timeout in seconds. (default: 60) [$SD_STORE_CLI_UPLOAD_HTTP_TIMEOUT]
   --download-timeout value  Specifies the download timeout in seconds. (default: 300) [$SD_STORE_CLI_DOWNLOAD_HTTP_TIMEOUT]
   --remove-timeout value    Specifies the removal timeout in seconds. (default: 300) [$SD_STORE_CLI_REMOVE_HTTP_TIMEOUT]
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

## Dependency

store-cli has dependency on ZStandard v1.4.8 (https://github.com/facebook/zstd)

To test in your local, please access the following website and execute 'make' locally.

https://github.com/facebook/zstd/releases/tag/v1.4.8

The site below can be useful when you're executing 'make' for zstd.

https://github.com/facebook/zstd#makefile
