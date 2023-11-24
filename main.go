package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/HamidAtPressPage/fresher/config"
	"github.com/HamidAtPressPage/fresher/runner3"
	"github.com/HamidAtPressPage/fresher/version"
)

func main() {
	//Handle flags.
	createConfig := flag.Bool("init", false, "Create a default configuration file in the current directory.")
	configFilePath := flag.String("config", "./"+config.DefaultConfigFileName, "Full path to the configuration file.")
	printConfig := flag.Bool("print-config", false, "Print the config file this app has loaded.")
	showVersion := flag.Bool("version", false, "Shows the version of the app.")
	tags := flag.String("tags", "", "Anything provided to 'go run' or 'go build' -tags.")
	verbose := flag.Bool("verbose", false, "Verbose logging.")
	flag.Parse()

	//If user just wants to see app version, print it and exit.
	//Not using log.Println() so that a timestamp isn't printed.
	if *showVersion {
		fmt.Println(version.V)
		os.Exit(0)
		return
	}

	//Log interpreted config file path.
	if *verbose {
		log.Println("fresher config:", *configFilePath)
	}

	//Check if user wants to create a default config file.
	if *createConfig {
		err := config.CreateDefaultConfig()
		if err != nil {
			log.Fatalln("Could not create default config file.", err)
			return
		}

		//Exit after creating default config to user can remove the -init flag.
		os.Exit(0)
		return
	}

	//Read and parse the config file at the provided path. The config file provides
	//runtime configuration of the app and contains settings that are rarely modified.
	// - If the --config flag is blank, the default value, a default config is used.
	// - If the --config flag has a path set, look for a file at the provided path.
	//    - If a file is found, parse it as config file and handle any errors.
	//    - If a file cannot be found, create a default config and save it to the path provided.
	err := config.Read(*configFilePath, *printConfig)
	if err != nil {
		log.Fatalln("Could not parse config file.", errors.Unwrap(err))
		return
	}

	//Handle overriding config with flags.
	if len(strings.TrimSpace(*tags)) > 0 {
		if !config.Data().UsingDefaults() {
			log.Println("WARNING! (main) Overriding Tags with provided -tags.")
		}
		config.Data().OverrideTags(*tags)
	}
	if *verbose {
		config.Data().OverrideVerbose(*verbose)
	}

	//Configure.
	err = runner3.Configure()
	if err != nil {
		log.Fatal("Error with configure", err)
		return
	}

	//Watch for changes to files.
	runner3.Watch()

	//Run.
	runner3.Start()
}
