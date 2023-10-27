# fresher
Automatic rebuilding and running of Go apps on source file changes.


# Purpose:
fresher is designed for use during development to remove the need to run `go run` each time a source code file is changed. Run `fresher` in place of `go run` and your binary will be rebuilt upon any .go file changes.


# Installing:
Run `go install github.com/c9845/fresher@latest`.


# Usage:
Run `fresher` in the same directory as you would run `go run`.

For more advanced usage, and customizing how `fresher` works, run `fresher -init` to create a config file in the current directory. The config file is pretty self-explainatory, however, see the [config file description](#configuration-file-details) below for more details.


# How `fresher` Works:
1. The directory tree, starting where fresher is run, is traversed recusively.
2. Each directory that contains at least one file with an applicable extension (i.e.: .go) is watched.
3. When a file is changed, `go build` is run and the built binary is then run. This is repeated upon each file change.


# Rewrite of `fresh`:
`fresher` is a rewrite of `github.com/gravityblast/fresh` (previously known as `github.com/pilu/fresh`) to improve the configuration options, improve, modernize, and document the code base, and improve performance. You can use `fresher` in the same manner as `fresh`.

#### Configuration:
- `fresh`'s configuration file used a custom format. `fresher` uses YAML. `fresher`'s configuration file is much easier to understand, use, and modify.
- `fresh` did not allow for using build tags. `fresher` allows for build tags via the GoTags configuration file field or the `-tags` flag being passed through.
- `fresh`'s configuration *will not* work with `fresher`. The configuration files are, however, somewhat similar and can be translated.

#### Improved Code:
- `fresher` modernizes the codebase using the latest third-party libraries, latest Go features and standard library, and implements Go modules.
- Code documentation is immensely better; `fresh` had near-zero code documentation. This should help with future development and maintance.

#### Improved Performance:
- `fresher` is a bit faster due to reduced build delays and faster rebuilds when a file change occurs during an ongoing build (the ongoing build is killed in `fresher`; `fresh` waited for the build to complete before rebuilding).
- File changes on Windows are handled better; duplicate file change events are caught preventing needless rebuilds.
- `fresher` uses one watcher goroutine instead of one goroutine per watched directory.


# Configuration File Details:

Create a configuration file with the `fresher -init` command.

Some configuration file fields can be overridden by flags to `fresher`.
- GoTags is overridden by `-tags`.
- Verbose is overridden by `-verbose`.

| Field | Description | Default|
|-------|-------------|--------|
| WorkingDir | The directory `fresher` should operate on. | . |
| EntryPoint | The relative path to the directory that holds the "main" package based off of the directory `fresher` is being run from. Typically this is "." meaning "main" is in the same directory as `fresher` is being run from. This really only needs to be used if your "main" package is in a subdirectory of your repo, such as "cmd/x". | . |
| TempDir | The name of the directory of of WorkingDir that `fresher` uses for storing the built binary and error logs. | "tmp" |
| ExtensionsToWatch | The types of files `fresher` will watch for changes. Typically just files used in a binary. | [".go", ".html"] |
| NoRebuildExtensions | The types of files `fresher` will just rerun, not rebuild, the binary on upon a file change occuring. Typically this includes files that are read and cached by a running binary (for example, HTML templates via the `html/template` package), but not included in the binary. Caution if you use embedded files! | [".html"] |
| DirectoriesToIgnore | Directories that will not be watched for changes, recursively. | ["tmp", "node_modules", ".git", ".vscode"]
| BuildDelayMilliseconds | The amount of time to wait after a file change event occurs before rebuilding the binary. A delay is useful for catching multiple saves happening in rapid succession. You should not need to set this higher than 300. | 300 | 
| BuildName | The name of the binary as built by `fresher`. This file is stored in TempDir. | fresher-build |
| BuildLogFilename | The name of the log file where errors from `go build` will be saved to. This file is stored in TempDir. | fresher-build-errors.log |
| GoTags | Anything you would provide to `go run -tags` or `go build -tags`. | "" |
| GoLdflags | Anything you would provide to `go build -ldflags`. | "-s -w" |
| GoTrimpath | If the `-trimpath` flag is provided to `go build`. | true |
| Flags | Any flags your binary defines using `flags` package. | "" |
| Verbose | If extra logging is provided while `fresher` is running. | false |


# FAQs: 

### Why not just use `air` (https://github.com/cosmtrek/air)?
In my testing and usage, `fresh` is much faster than `air` at responding to file change events and rebuilding/rerunning the binary. Although `air` is a more-or-less a fork of `fresh`, it does not improve performance. It seems like `air` is much "heavier" and there is more of a focus on tooling (for building `air`, i.e.: testing, CI, tools, etc.) rather than the underlying functionality.

### Why is the binary built and run instead of just `go run`-ed?
The process of building then running was inherited from `fresh`.  It isn't clear why this was done.  

We assume the build & run method was used since:
- Then the entrypoint file (i.e.: main.go) does not need to be provided (`go run main.go` vs. `go build`).
- Rerunning an already built binary is faster than running `go run` if a file was changed that doesn't require a rebuild (see NoRebuildExtensions).
- Possibly easier inspecting of build errors.
- Is `go run` really any faster than `go build`?


# Contributing:
- `gofmt` is required.
- `staticcheck` is required and must return no warnings.
- Try to keep code style as similar as possible.
- Comment lines are ~85 characters long.
- All tests must pass.
- Total code coverage isn't of utmost performance, although raising coverage is nice.
- Performance is important. Performance (speed of recognizing file change events and rebuilding) should never degrade.
- Code comments/documentation should always try to answer "why" something was done. Expect the reader to not understand much.
