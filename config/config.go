/*
Package config handles configuration of the app. The configuration can simply be
a default, or a config file can be provided. The config data is used for low-level
settings of the app and can be used elsewhere in this app.

The config file is in YAML format for easy readability. However, the file does not
need to end in the yaml extension. YAML is used since it allows for adding comments
to the config file (JSON does not). Having comments is really nice for providing some
additional info in the config file.

This package must not import any other packages from within this app to prevent
import loops (besides minor utility packages).

---

Create a default config file in the current working directory using the -init flag.

A config is handled one of three ways:
  - If a file exists at the path, it is attempted to be parsed as a valid config file.
  - If a file does not exists at the path, the built-in defaults are used after a
    warning is shown about the missing file.
  - If the path is blank, the default config is used.

---

When adding a new field to the config file:
  - Add the field to the File type below.
  - Determine any default value(s) for the field and set it in newDefaultConfig().
  - Set validation in validate().
  - Document the field as needed (README, other documentation).

Try to keep the organization/order of the config fields the same between the config
file templates, the type defined below, validation, and diagnostics.
*/
package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/c9845/fresher/version"
	"gopkg.in/yaml.v3"
)

// DefaultConfigFileName is the typical name of the config file.
const DefaultConfigFileName = "fresher.conf"

// File defines the list of configuration fields. The value for each field will be
// set by a default or read from a config file. The config file is typically stored
// in the same directory as the executable.
//
// Don't use uints! If user provided a negative number an ugly error message is
// kicked out. We would rather check for the negative number here and provide a
// nicer error message.
//
// Struct tags are needed for working with yaml.v3 package, otherwise the package
// expects fields to start with lower case characters. However, if we lower cased all
// the struct field names, then we wouldn't be able to access those fields in other
// packages.
//
// If adding or updating a field here, make sure to document it in README.md!
type File struct {
	//WorkingDir is the path to the root of your repository. All files in or within
	//subdirectories of this directory, recursively, will be watched for changes.
	//
	//Typically this is set to "." since your terminal, and thus where fresher is
	//being run from, is already set to the root directory of your repo.
	WorkingDir string `yaml:"WorkingDir"`

	//EntryPoint is the path to your "package main" file, the root file of your app
	//that you want to build.
	//
	//If you have a "main.go" file in your local directory from which you are running
	//fresher, you can leave this as ".". However, if your "package main" is in any
	//other path, specifically a subdirectory of your repo, you must provide the
	//relative path to it here (i.e: "cmd/app/app.go").
	EntryPoint string `yaml:"EntryPoint"`

	//TempDir is the directory off of WorkingDir where fresher will store the built
	//binary, that will be run, and error logs.
	TempDir string `yaml:"TempDir"`

	//ExtensionsToWatch is the list of file extensions to watch for changes, typically
	//.go and .html (if building a web app).
	ExtensionsToWatch []string `yaml:"ExtensionsToWatch"`

	//NoRebuildExtensions is the list of extensions that the binary will be restarted
	//on when file changes occur, but the binary won't be rebuilt. Any extension
	//listed here should, obviously, be listed in ExtensionsToWatch as well.
	//
	//For example, if an .html file is changed, the binary would need to be restarted
	//since HTML files are typically stored in memory (using html/templates) when the
	//binary is first started.
	NoRebuildExtensions []string `yaml:"NoRebuildExtensions"`

	//DirectoriesToIgnore is the list of directories that won't be watched for file
	//change events. Typically directories such as .git, node_modules, etc.
	DirectoriesToIgnore []string `yaml:"DirectoriesToIgnore"`

	//BuildDelayMilliseconds is the delay between a file change event occuring and
	//`go build` being run. This delay is helpful to prevent unnecessary buildng when
	//multiple file change events occur in quick succession.
	//
	//This was inherited from the "github.com/gravityblast/fresh" and may not be
	//needed any longer since running `go build`s will be cancelled if a new file
	//change event occurs.
	BuildDelayMilliseconds int64 `yaml:"BuildDelayMilliseconds"`

	//BuildName is the name of the binary output by `go build` and saved to TempDir.
	BuildName string `yaml:"BuildName"`

	//BuildLogFilename is the name of file saved in TempDir where build errors will
	//be logged to. This file will contain output from `go build` and is useful for
	//analyzing errors rather then looking at output in terminal.
	BuildLogFilename string `yaml:"BuildLogFilename"`

	//GoTags is anything provided to `go run` or `go build` -tags flag.
	//
	//Any tags provided in the config, from file or defaults, are overridden by
	//anything provided to the -tags flag provided to fresher. This was done to
	//alleviate the need to always edit a config file for handling -tags changes.
	GoTags string `yaml:"GoTags"`

	//GoLdflags is anything provided to `go build` -ldflags flag.
	//See https://pkg.go.dev/cmd/link for possible options.
	GoLdflags string `yaml:"GoLdflags"`

	//GoTrimpath determines if the -trimpath flag should be passed to `go build`.
	//Typically this isn't needed since the built binary won't be distributed since
	//fresher is designed for development use only.
	//See https://pkg.go.dev/cmd/go#:~:text=but%20still%20recognized.)%0A%2D-,trimpath,-remove%20all%20file.
	GoTrimpath bool `yaml:"GoTrimpath"`

	//Flags is the list of flags you provide to your binary or `go run`, such as
	//`go run main.go -flag1="test"`.
	Flags string `yaml:"Flags"`

	//Verbose causes fresher to output more logging. Use for diagnostics when
	//determining which files/directories/extensions are being watched and when file
	//change events are occuring.
	Verbose bool `yaml:"Verbose"`

	//usingBuiltInDefaults is set to true only when File isn't actually read from a
	//file and we are using the built in defaults instead. This is used to reduce
	//diagnostic output (i.e.: path to config file) when a config file wasn't used
	//since if a config file wasn't used, there is no path to log out!
	usingBuiltInDefaults bool `yaml:"-"`
}

// parsedConfig is the data parsed from the config file. This data is stored so that
// we don't need to reparse the config file each time we need a piece of data from it.
// This is not exported so that changes cannot be made to the parsed data as easily.
// Use the Data() func to get the data for use elsewhere.
var parsedConfig File

// newDefaultConfig returns a File with default values set for each field.
func newDefaultConfig() (f File) {
	f = File{
		WorkingDir:             ".",                                                //local directory fresher is being run from
		EntryPoint:             ".",                                                //we assume the "package main" is also in the directory fresher is being run from
		TempDir:                "tmp",                                              //most likely an unused directory
		ExtensionsToWatch:      []string{".go", ".html"},                           //common files to watch
		NoRebuildExtensions:    []string{".html"},                                  //don't need to rebuild, unless they are embedded
		DirectoriesToIgnore:    []string{"tmp", "node_modules", ".git", ".vscode"}, //save to ignore these directories
		BuildDelayMilliseconds: 100,                                                //100 is "instant" enough but helps catch CTRL+S being hit rapidly.
		BuildName:              "fresher-build",                                    //could really be anything.
		BuildLogFilename:       "fresher-build-errors.log",                         //could really be anything.
		GoTags:                 "",                                                 //will be overridden by flag to fresher.
		GoLdflags:              "-s -w",                                            //probably unnecessary since the built binary shouldn't be used for production or distribution.
		GoTrimpath:             true,                                               //probably unnecessary since the built binary shouldn't be used for production or distribution.
		Flags:                  "",                                                 //
		Verbose:                false,                                              //will be overridden by flag to fresher.

		usingBuiltInDefaults: true,
	}
	return
}

// CreateDefaultConfig creates a config file in the current directory with the default
// configuration. This is mostly used to create a template config file to modify.
// Used/called by the -init flag.
func CreateDefaultConfig() (err error) {
	//Get default config.
	cfg := newDefaultConfig()

	//Get path to save config to.
	path := filepath.Join(".", DefaultConfigFileName)

	//Check if a config file already exists at this path to prevent overwriting it.
	_, err = os.Stat(path)
	if err == nil {
		log.Printf("WARNING! (config) Config file already exists at %s, skipping creation. Remove the -init flag.", path)
		return nil
	}

	//Save the config to a file.
	err = cfg.write(path)
	if err != nil {
		return
	}

	log.Printf("WARNING! (config) Config file created with defaults at %s, remove the -init flag in the future.", path)
	return
}

// Read handles reading and parsing the config file at the provided path and saving
// it to the package's parsedConfig variable for future use. The parsed data is
// sanitized and validated. The print argument is used to print the config as it was
// read/parsed and as it was understood after sanitizing, validating, and handling
// default values.
//
// If a config file is not found at the given path, a warning is shown and the
// built-in default config is used instead. Use -init to create a default config file.
func Read(path string, print bool) (err error) {
	//Clean the path to remove anything odd.
	path = filepath.Clean(path)

	//Handle path to config file.
	var cfg File
	if strings.TrimSpace(path) == "" {
		//The path provided for the config file flag is blank (-config=""). This
		//should never really happen since the flag defines a default value (a file
		//named licensekeys.conf in the working directory where binary is being run from).
		//This simply catches instances where user provides -config="" for some odd
		//reason. Since a path was not provided, we cannot save a config file
		//anywhere. In this case, we just use an "in memory" config that uses default
		//values.

		//Get default config.
		cfg = newDefaultConfig()

	} else if _, err = os.Stat(path); os.IsNotExist(err) {
		//A path was provided to the config file flag but a file does not exist at
		//the given path. A config file will be saved at the provided path with
		//default values.
		//
		//The path provided to the -config flag could be a user provided value or a
		//default value (i.e.: flag wasn't provided at all).

		//Get default config.
		cfg = newDefaultConfig()

		//Unset the file not found error.
		err = nil

	} else {
		//A path was provided to the config file flag and a file exists at the given
		//path. Parse the file as a config file. If the file isn't a valid config
		//file, error out, otherwise continue running the app.

		//Read the file at the path.
		f, innerErr := os.ReadFile(path)
		if innerErr != nil {
			return innerErr
		}

		//Parse the file as yaml.
		innerErr = yaml.Unmarshal(f, &cfg)
		if innerErr != nil {
			return innerErr
		}

		//Print the config, if needed, as it was parsed from the file. This logs
		//out the config fields with the user provided data before any validation.
		if print {
			log.Println("***PRINTING CONFIG AS PARSED FROM FILE***")
			cfg.print(path)
		}
	}

	//Validate & sanitize the data since it could have been edited by a human.
	err = cfg.validate()
	if err != nil {
		return err
	}

	//Save the config to this package for use elsewhere in the app.
	parsedConfig = cfg

	//Print the config, if needed, as it was sanitized and validated. This logs out
	//the config as it was understood by the app and some changes may have been made
	//(for example, user provided an invalid value for a field and a default value
	//was used instead). This also prints out the config if it was created or if the
	//config path was blank and a default config was used instead.
	//Always exit at this point since printing config is just for diagnostics.
	if print {
		log.Println("***PRINTING CONFIG AS UNDERSTOOD BY FRESHER***")
		parsedConfig.print(path)
		os.Exit(0)
		return
	}

	return
}

// write writes a config to a file at the provided path.
func (conf *File) write(path string) (err error) {
	//Marshal to yaml.
	y, err := yaml.Marshal(conf)
	if err != nil {
		return
	}

	//Create the file.
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	//Add some comments to config file so a human knows it was generated, not
	//written by a human.
	file.WriteString("#Generated config file for fresher.\n")
	file.WriteString("#Generated at: " + time.Now().UTC().Format(time.RFC3339) + "\n")
	file.WriteString("#Version: " + version.V + "\n")
	file.WriteString("#This file is in YAML format.\n")
	file.WriteString("\n")
	file.WriteString("#***Do not delete this file!***\n")
	file.WriteString("\n")
	file.WriteString("#***On Windows, when providing a path, use forward slashes in place of a back slashes and surround the path in double quotes!***\n")
	file.WriteString("\n")

	//Write config to file.
	_, err = file.Write(y)
	return
}

// validate handles sanitizing and validation of a config file's data.
func (conf *File) validate() (err error) {
	//Get defaults to use for cases when user provided invalid input.
	defaults := newDefaultConfig()

	//Clean all paths, regardless of if they are used.
	conf.WorkingDir = filepath.Clean(filepath.ToSlash(strings.TrimSpace(conf.WorkingDir)))
	conf.EntryPoint = filepath.Clean(filepath.ToSlash(strings.TrimSpace(conf.EntryPoint)))
	conf.TempDir = filepath.Clean(filepath.ToSlash(strings.TrimSpace(conf.TempDir)))

	//Make sure working directory is set. This should just be "." in most cases since
	//the working directory is the directory where "fresher" is being run.
	conf.WorkingDir = filepath.FromSlash(strings.TrimSpace(conf.WorkingDir))
	if conf.WorkingDir == "" {
		return errors.New("config: WorkingDir not set. Typically this should be set to \".\"")
	}

	//Make sure entry point directory is set. This might be "." or any other path.
	conf.EntryPoint = strings.TrimSpace(conf.EntryPoint)
	if conf.EntryPoint == "" {
		return errors.New("config: EntryPoint not set. Typically this should be set to \".\"")
	}

	//Make sure temp directory is somewhat valid looking.
	conf.TempDir = filepath.FromSlash(strings.TrimSpace(conf.TempDir))
	if conf.TempDir == "" {
		conf.TempDir = defaults.TempDir
		log.Println("WARNING! (config) TempDir not provided, defaulting to " + conf.TempDir + ".")
	}

	//Make sure each extension to watch is only provided once.
	validExtensionsToWatch := []string{}
	for _, extension := range conf.ExtensionsToWatch {
		extension = strings.TrimSpace(extension)

		if !strings.HasPrefix(extension, ".") {
			log.Println("WARNING! (config) ExtensionsToWatch " + extension + " missing leading period, added.")
		}

		if slices.Contains(validExtensionsToWatch, extension) {
			log.Println("WARNING! (config) ExtensionsToWatch duplicate " + extension + ", ignored.")
			continue
		}

		validExtensionsToWatch = append(validExtensionsToWatch, extension)
	}
	conf.ExtensionsToWatch = validExtensionsToWatch

	//Make sure at least one extension to watch was given. If no extensions were given,
	//then we don't know what files to watch for changes!
	if len(conf.ExtensionsToWatch) == 0 {
		conf.ExtensionsToWatch = defaults.ExtensionsToWatch
		log.Printf("WARNING! (config) ExtensionsToWatch not provided, defaulting to %s.", conf.ExtensionsToWatch)
	}

	//Make sure any no-rebuild extensions are also watched extensions.
	validNoRebuildExtensionss := []string{}
	for _, extension := range conf.NoRebuildExtensions {
		extension = strings.TrimSpace(extension)

		if !strings.HasPrefix(extension, ".") {
			log.Println("WARNING! (config) NoRebuildExtensions " + extension + " missing leading period, added.")
		}

		if slices.Contains(validNoRebuildExtensionss, extension) {
			log.Println("WARNING! (config) NoRebuildExtensions duplicate " + extension + ", ignored.")
			continue
		}

		if !slices.Contains(conf.ExtensionsToWatch, extension) {
			log.Println("WARNING! (config) NoRebuildExtensions extension " + extension + " not included in ExtensionsToWatch, added.")
			conf.ExtensionsToWatch = append(conf.ExtensionsToWatch, extension)
		}

		validNoRebuildExtensionss = append(validNoRebuildExtensionss, extension)
	}
	conf.NoRebuildExtensions = validNoRebuildExtensionss

	//Remove duplicate directories to ignore and sanitize each.
	validDirectoriesToIgnore := []string{}
	for _, dir := range conf.DirectoriesToIgnore {
		//Sanitize.
		dir = strings.TrimSpace(dir)
		dir = filepath.Clean(dir)

		//We don't check if a directory actually exists. Who cares if a directory
		//listed in the config file doesn't actually exists in the repo.

		if slices.Contains(validDirectoriesToIgnore, dir) {
			log.Println("WARNING! (config) Duplicate directory " + dir + " in DirectoriesToIgnore.")
			continue
		}

		validDirectoriesToIgnore = append(validDirectoriesToIgnore, dir)
	}
	conf.DirectoriesToIgnore = validDirectoriesToIgnore

	//Validate some other stuff.
	if conf.BuildDelayMilliseconds < 0 {
		conf.BuildDelayMilliseconds = defaults.BuildDelayMilliseconds
		log.Printf("WARNING! (config) BuildDelayMilliseconds must be greater then 0, defaulting to %d.", conf.BuildDelayMilliseconds)
	}

	if strings.TrimSpace(conf.BuildName) == "" {
		conf.BuildName = defaults.BuildName
		log.Println("WARNING! (config) BuildName was not given, defaulting to " + conf.BuildName + ".")
	}

	if strings.TrimSpace(conf.BuildLogFilename) == "" {
		conf.BuildLogFilename = defaults.BuildLogFilename
		log.Println("WARNING! (config) BuildLogFilename was not given, defaulting to " + conf.BuildLogFilename + ".")
	}

	return
}

// print logs out the configuration file. This is used for diagnostic purposes.
// This will show all fields from the File struct, even fields that the provided
// config file omitted (except nonPublishedFields).
func (conf File) print(path string) {
	//Don't print paths when the default built-in config is in use. There aren't any
	//paths since config wasn't read from file!
	if !conf.usingBuiltInDefaults {
		//Full path to the config file, so if file is in same directory as the
		//executable and -config flag was not provided we still get the complete path.
		pathAbs, _ := filepath.Abs(path)

		log.Println("Path to config file (flag):", path)
		log.Println("Path to config file (absolute):", pathAbs)
	}

	//Print out config file stuff (actually from parsed struct).
	x := reflect.ValueOf(&conf).Elem()
	typeOf := x.Type()
	for i := 0; i < x.NumField(); i++ {
		if typeOf.Field(i).IsExported() {
			fieldName := typeOf.Field(i).Name
			value := x.Field(i).Interface()
			log.Println(fieldName+":", value)
		}
	}
}

// Data returns the package level saved config. This is used in other packages to
// access the parsed config file.
func Data() *File {
	return &parsedConfig
}

// IsTempDir returns true if the given path represents the same directory as TempDir.
// We use absolute paths here since we want to be certain if the path given matches
// the same underlying directory as given in TempDir.
func (conf *File) IsTempDir(path string) (yes bool, err error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	fullTempDirPath, err := filepath.Abs(conf.TempDir)
	if err != nil {
		return
	}

	if pathAbs == fullTempDirPath {
		return true, nil
	}

	return false, nil
}

// IsDirectoryToIgnore returns true if the given path is in the DirectoriesToIgnore.
func (conf *File) IsDirectoryToIgnore(path string) bool {
	//not using slices.Contains() because of the path may be within a directory to
	//ignore, and therefore we need to use HasPrefix().
	for _, d := range conf.DirectoriesToIgnore {
		if strings.HasPrefix(path, d) {
			return true
		}
	}

	return false
}

// IsRebuildExtension returns true if the given extension is not in the
// NoRebuildExtensions list.
func (conf *File) IsRebuildExtension(extension string) bool {
	return !slices.Contains(conf.NoRebuildExtensions, extension)
}

// IsExtensionToWatch returns true if the given path contains an extension we should
// watch for changes.
func (conf *File) IsExtensionToWatch(extension string) bool {
	return slices.Contains(conf.ExtensionsToWatch, extension)
}

// UsingDefaults returns true is usingbuildInDefaults is set to true.
func (conf *File) UsingDefaults() bool {
	return conf.usingBuiltInDefaults
}

// OverrideTags sets the Tags field to t. This is used when the -tags flag was provided
// and overrides the value stored in parsedConfig's Tags field. This is useful for
// changing tags without having to edit the config file (if it exists) each time.
func (conf *File) OverrideTags(t string) {
	conf.GoTags = strings.TrimSpace(t)
}

// OverrideVerbose sets the Verbose field to v. This is used when the -verbose
// flag was provided and overrides the value stored in teh parsedConfig's Verbose
// field. This is useful for when (1) you aren't using a config file (i.e.: the default
// running method of fresher), or (2) you have a config file and just want some extra
// logging on a case-by-case basis.
func (conf *File) OverrideVerbose(v bool) {
	conf.Verbose = v
}
