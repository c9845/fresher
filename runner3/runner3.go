/*
Package runner3 handles building and running the binary when a file has been modified.

How watching/building/running works:
  - The directory tree starting at the root working directory is walked (recursively).
  - If the directory is not ignored, per the config file settings, an fsnotify watcher
    is set on each directory to watch for changes to files within that directory.
  - When a file change event occurs, the file is checked to see if it has a watched
    extension. If yes, the binary is rebuilt and/or rerun as needed.
*/
package runner3

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/HamidAtPressPage/fresher/config"
	"github.com/fsnotify/fsnotify"
)

// Define communication channels.
var (
	//eventsChan relays file change events from the file change watcher to the binary
	//builder. Messages are in the format `"file: DELETE|MODIFY|...`.
	//See: https://pkg.go.dev/github.com/fsnotify/fsnotify#Event.String
	eventsChan = make(chan fsnotify.Event, 1)

	//stopChan is for terminating the built and running binary when the binary is
	//rebuilt. This prevents multiple copies of the binary from running concurrently.
	stopChan = make(chan bool)

	//killBuildingChan is used to signal to build() that the `go build...` command should
	//be terminated. This is used when another file change event has occured while
	//build() is running that will just cause build() to run again. There is no sense
	//in completing the currently running build since build() will just be called
	//again immediately after completing. Killing off the running build just saves a
	//bit of time.
	killBuildingChan = make(chan bool, 1)
)

// Configure handles some initialization steps before watching for file changes and
// handling building and running the binary.
func Configure() (err error) {
	//Set up logging.
	events = newLogger("fresher", "blue")
	warn = newLogger("fresher", "yellow")
	errs = newLogger("fresher", "red")

	//Set the number of maximum file descriptors that can be opened by this process.
	//This is needed for watching a HUGE amount of files. Windows is not applicable.
	err = setRLimit()
	if err != nil {
		return
	}

	//Create the temp directory to store the build binary and error logs.
	err = os.MkdirAll(config.Data().TempDir, 0755)
	if err != nil {
		return
	}

	//Debug logging.
	warn.Verbosef("Watching extensions: %s", config.Data().ExtensionsToWatch)
	warn.Verbosef("Ignoring directories: %s", config.Data().DirectoriesToIgnore)

	return
}

// Watch handles setting up the watcher of file changes. The watcher is populated with
// a list of directories to watch, not individual files. Some directories are ignored
// per the config file field DirectoriesToIgnore.
//
// When a file change event occurs, the event is sent on the eventsChan which will be
// recevied in start() and is used to trigger the binary being built via build().
func Watch() (err error) {
	//Initialize the watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	//Add paths to watcher of the directories to watch for file changes. We watch
	//directories, not individual files, for changes.
	//
	//This works by walking the directory starting at the working directory, typically
	//the directory fresher is being run in, checking if each directory should be
	//watched or ignored (as set in config file), and adding the directory to the
	//watcher.
	err = filepath.WalkDir(config.Data().WorkingDir, func(path string, d fs.DirEntry, err error) error {
		//Handle errors related to the path. See fs.WalkDirFunc for more info.
		if err != nil {
			return err
		}

		//Only watch directories, not individual files.
		if !d.IsDir() {
			return nil
		}

		//Ignore directory if it is the temp directory where built binaries are stored
		//before running. No need to watch this directory since it stores temp data
		//from fresher.
		yes, err := config.Data().IsTempDir(path)
		if err != nil {
			return err
		}
		if yes {
			return fs.SkipDir
		}

		//Ignore directory if it is in list of ignored directories. Ignored directories
		//listed in config file are based off of the WorkingDir. The path in the
		//WalkDirFunc here is also based off of the WorkingDir, so therefore we can
		//easily compare without having to handle absolute paths.
		if config.Data().IsDirectoryToIgnore(path) {
			warn.Verbosef("IGNORING %s", path)

			return fs.SkipDir
		}

		//Add path to watcher.
		events.Verbosef("Watching %s", path)
		err = watcher.Add(path)
		return err
	})
	if err != nil && err != fs.SkipDir {
		return
	}

	//Watch for file change events. When an event does occur, make sure it is a
	//file write (not CHMOD or something else) and that the file that was changed has
	//an extension that we watch for (i.e.: no sense in sending events to rebuild
	//binary if a .docx file was changed).
	go func() {
		//Handle double-save events that can sometimes occur. Usually due to "save
		//new file and rename" method of saving files by text editors/OSes. This is
		//particularly helpful on Windows as duplicate events occur for each file
		//save a human initiates.
		//
		//This works by setting a timer when a file change event occurs (see time.Reset
		//below) when a file change event occurs. While the timer is running, before
		//it expires, other file change events are still received. However, only the
		//last event is "remembered". After the timer expires, the "remembered" last
		//event is sent on the events channel causing the rebuild and/or rerun to
		//occur.
		//
		//Taken from: https://github.com/fsnotify/fsnotify/issues/122#issuecomment-1065925569
		//
		//Note the immediately below NewTimer related code. This just initiates the
		//timer and reads the first expiration so the timer can be reset when events
		//occur.
		var lastEvent fsnotify.Event
		timer := time.NewTimer(time.Millisecond)
		<-timer.C

		for {
			select {
			case err := <-watcher.Errors:
				if err != nil {
					errs.Printf("watcher error %s", err)
				}

			case event := <-watcher.Events:
				//Ignore event on certain events.
				if event.Op == fsnotify.Chmod {
					continue
				}

				//Skip sending event if a non-watched file is changed.
				if !config.Data().IsExtensionToWatch(filepath.Ext(event.Name)) {
					continue
				}

				//Store the event and wait a short while to catch duplicate events.
				lastEvent = event
				timer.Reset(time.Millisecond * 50)

			case <-timer.C:
				eventName := lastEvent.Name
				eventType := lastEvent.Op.String()

				events.Verbosef("Sending Event... %s (%s)", eventName, eventType)

				//Cause binary to be rebuilt and/or rerun.
				eventsChan <- lastEvent

				//Check if binary is currently being built and stop the build if this
				//event will just result in a rebuild. This saves a bit of time since
				//we don't build the binary twice (once is ongoing and again for the
				//new event) and have to wait for the first build to complete before
				//the second build starts.
				//
				//This is not checked in start() since start blocks when build() is
				//running and thus will not be able to receive a new event until build()
				//is complete, therefore building can never be killed!
				rebuildRequired := config.Data().IsRebuildExtension(filepath.Ext(eventName))
				if buildCmdRunning && rebuildRequired {
					killBuildingChan <- true
				}
			}

		}
	}()

	//Watcher is set up to watch for changes in directories.
	//goroutine watching for file change events will continue running.

	return
}

// start watches for file change events and runs the commands to build and run the
// binary.
func start() {
	//Is fresher running the binary. If yes, and a build error occurs, the currently
	//running binary won't be stopped. If no, then the binary isn't running and this
	//is most likely the first time fresher has been run, therefore just exit fresher
	//on a build error.
	started := false

	//Wait for file change events to rebuild and rerun the binary. This waits for
	//file change events sent on the eventsChan as set up in Watch().
	go func() {
		for {
			//Get event.
			event := <-eventsChan
			eventName := event.Name
			eventType := event.Op.String()
			events.Printf("Got Event... %s (%s)", eventName, eventType)

			//Track if build is successful so we know to stop watching and building.
			buildSuccessful := false

			//Determine if we need to rebuild the binary. We really only need to
			//rebuild if a .go file changes (unless the binary is using embedded
			//files). This is simply a performance improver since we do not need to
			//rebuild the binary if, say, an HTML file is changed.
			rebuildRequired := config.Data().IsRebuildExtension(filepath.Ext(eventName))
			if rebuildRequired {
				//Binary should be rebuilt.

				//Get build delay so that we don't rebuild too fast. This helps improve
				//performance a bit when multiple file events occur in rapid succession
				//since the binary won't be built, the build cancelled (see
				//killBuildingChan channel), then the build starting again, etc.
				//
				//The build delay should be low enough not to induce too much latency
				//before building but long enough to catch rapid file saves.
				delay := time.Duration(config.Data().BuildDelayMilliseconds) * time.Millisecond
				events.Verbosef("Waiting %s before rebuilding...", delay)
				time.Sleep(delay)
				events.Verbosef("Waiting %s before rebuilding...done", delay)

				//Clear the error log since we are rebuilding the binary.
				err := deleteBuildErrorsLog()
				if err != nil && !os.IsNotExist(err) {
					errs.Printf("Error deleting build log %s", err)
					//not exiting on error since this isn't an end-of-the-world event.
				}

				//Build the binary. Same as running `go build`.
				err = build(event)
				if err == errBuildKilled {
					buildSuccessful = false
				} else if err != nil {
					errs.Printf("Build Failed %s", err)
					if !started {
						//Build failed and the binary never stared running, exit fresher.
						//This should only occur when fresher just starts and builds
						//the binary for the first time.
						os.Exit(1)
					}
				} else {
					buildSuccessful = true
				}
			}

			//Handle logging when binary was previously built successfully but failed
			//building this time. The currently running binary will continue running.
			if rebuildRequired && !buildSuccessful {
				errs.Printf("Rebuild failed or killed, previous build still running.")
				continue
			}

			//Handle logging for starting of the built binary. Have to handle binary
			//being built first time, being rebuild, or existing binary just being
			//rerun.
			if started {
				if !rebuildRequired {
					warn.Verbosef("Rerunning existing binary, file with no rebuild extension changed...")
				} else {
					events.Verbosef("Running rebuilt binary...")
				}

				stopChan <- true
			} else {
				events.Verbosef("Running first build of binary...")
			}

			//Run the newly built binary or restart a previously built binary if a
			//file was changed that doesn't require a rebuild (i.e.: html).
			run()

			//Add logging line to separate fresher logging output from built
			//binary's logging output.
			events.Printf(strings.Repeat("-", 50))

			//Note that binary is started. This way if a subsequent build fails, the
			//running binary won't be stopped.
			started = true
		}
	}()
}

// deleteBuildErrorsLog deletes the build errors log file located at the path noted in
// the config file field BuildLogFilename within the TempDir. Each time a new binary
// is built the error log is deleted and recreated if another error occurs.
func deleteBuildErrorsLog() (err error) {
	pathToFile := filepath.Join(config.Data().TempDir, config.Data().BuildLogFilename)
	err = os.Remove(pathToFile)
	return
}

// buildCmdRunning is used to monitor the state of whether or not the `go build`
// command is running. This is set in build() when .Start() is called and reset when
// after .Wait() stops blocking.
//
// This is read in start() to check if the binary is currently being built when a new
// file change event occurs and determines if a message is sent on the killBuildingChan.
var buildCmdRunning bool = false

// errors returned from build()
var (
	//errBuildFailed is returned when a build fails.
	errBuildFailed = errors.New("build failed")

	//errBuildKilled is returned when a build is killed in the middle of building due
	//to a message on the killBuildingChan channel. This isn't really an error since
	//the binary will just be rebuilt (similar error in usage as fs.SkipDir).
	errBuildKilled = errors.New("build killed")
)

// build builds the binary. This runs `go build` and outputs a binary to the temp
// directory noted in the config file.
//
// A string is returned only upon an stderr output when an stderr occurs in `go build`.
// True is returned when build is successful.
//
// build() is called in start().
func build(event fsnotify.Event) (err error) {
	//Debugging.
	eventName := event.Name
	eventType := event.Op.String()

	//Get path and name to output built binary as. This is a file located in the
	//temp directory.
	pathToBuiltBinary := getPathToBuiltBinary()

	//Build arguments passed to "go" command.
	args := []string{
		"build",
		"-o", pathToBuiltBinary,
	}

	//Handle other go build flags.
	if len(config.Data().GoTags) > 0 {
		args = append(args, "-tags", config.Data().GoTags)
	}

	if len(config.Data().GoLdflags) > 0 {
		args = append(args, "-ldflags", config.Data().GoLdflags)
	}

	if config.Data().GoTrimpath {
		args = append(args, "-trimpath")
	}

	//Get path to entry point of app. This is typically just the repository root,
	//but could be a subdirectory as well.
	entryPoint := config.Data().EntryPoint

	//Add the entry point to build the binary from.
	args = append(args, entryPoint)

	//Initialize the command, but do not run it.
	buildStartTime := time.Now()
	cmd := exec.Command("go", args...)
	if config.Data().Verbose {
		events.Verbosef("Building... %s %s", "go", strings.Join(args, " "))
	} else {
		events.Printf("Building... %s (%s)", eventName, eventType)
	}

	//Set up logging for when the command runs. We want to capture the output logging
	//and output it to the user running fresher. This is so the user can see any output
	//from running `go build` to diagnose issues.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	//Start handler to kill builds if needed. This is used to stop builds when another
	//file change will cause build() to be run again. Since we are just going to
	//build the binary again almost instantly after this build completes, we can kill
	//off the running build.
	//
	//We use cancelKiller to stop the goroutine once a build completes successfully,
	//this way we don't have this goroutine sitting around needlessly after a build
	//is completed.
	//
	//buildKilled is used to return a specific error when a build is killed.
	cancelKiller := make(chan bool, 1)
	buildKilled := false
	go func() {
		select {
		case x := <-killBuildingChan:
			if x {
				//Not using errs/warn/events logger here on purpose. I think it causes
				//a panic when fresher is left running, a computer sleeps, and then
				//wakes back up. Using log.Println() seems to alleviate the issue.
				log.Println("Building...killed")

				err := cmd.Process.Kill()
				if err != nil {
					log.Printf("Killing build error %s", err)
				}

				buildKilled = true
			}

		case <-cancelKiller:
			//Terminate this goroutine since build was completed. This was we don't
			//end up with endlessly running goroutines.
		}
	}()

	//Run the command, go build...
	buildCmdRunning = true
	err = cmd.Start()
	if err != nil {
		return
	}

	//Copy output for stdout to fresher's stdout. This way user sees output from
	//building.
	_, err = io.Copy(os.Stdout, stdout)
	if err != nil {
		return
	}

	//Capture stderr since it might have a bunch of diagnostic info about why built
	//failed. Stderr is saved to error file so that it is easier to inspect then
	//reading in terminal output.
	errBuf, err := io.ReadAll(stderr)
	if err != nil {
		errs.Printf("Error capturing stderr %s", err)
	}

	//Wait for command to finish. Have to handle build being killed by us!
	err = cmd.Wait()
	if err != nil && buildKilled {
		return errBuildKilled
	} else if err != nil {
		return
	}

	//Build is complete. Stop the goroutine the monitors if the build should be stopped
	//while it is running (in case another file change event occured while building
	//that would just cause the binary to be built again anyway).
	buildCmdRunning = false
	cancelKiller <- true

	//If an error occured, write the output to a log file. There could be useful info
	//such as stack traces or other logging to identify issue in this error.
	if len(errBuf) > 0 {
		saveBuildErrorsLog(string(errBuf))
		return errBuildFailed
	}

	//Extra logging.
	events.Verbosef("Building... %s %s (Took %s)", "go", strings.Join(args, " "), time.Since(buildStartTime))

	//Build was successful. Binary is now located in temp dir.
	return
}

// getPathToBuiltBinary returns the path to where the build binary will be saved.
// Basically, append BuildName to TempDir and add .exe if needed.
func getPathToBuiltBinary() string {
	path := filepath.Join(config.Data().TempDir, config.Data().BuildName)
	if runtime.GOOS == "windows" && filepath.Ext(path) != ".exe" {
		path += ".exe"
	}

	return path
}

// saveBuildErrorsLog saves the stderr output from `go build` when build() is called
// to a file. This file is deleted each time a build is attempted via
// deleteBuildErrorsLog which is called in start().
func saveBuildErrorsLog(message string) {
	//Get path to log file.
	pathToFile := filepath.Join(config.Data().TempDir, config.Data().BuildLogFilename)

	//Create the file.
	f, err := os.Create(pathToFile)
	if err != nil {
		errs.Printf("Could not create log file %s", err)
		//not exiting on error since we don't do anything with error anyway.
	}
	defer f.Close()

	//Write to file
	_, err = f.WriteString(message)
	if err != nil {
		errs.Printf("Could not write log file %s", err)
		//not exiting on error since we don't do anything with error anyway.
	}
}

// run runs the binary build in build().
//
// run() is called in start().
func run() {
	//Get path to built binary.
	pathToBuiltBinary := getPathToBuiltBinary()

	//Build arguments, i.e.: flags, to provide to binary.
	args := []string{
		config.Data().Flags,
	}

	//Initialize the command, but do not run it.
	cmd := exec.Command(pathToBuiltBinary, args...)
	if config.Data().Verbose {
		events.Printf("Running... %s %s", pathToBuiltBinary, strings.Join(args, " "))
	} else {
		events.Printf("Running...")
	}

	//Set up logging for when the command runs. We want to capture the output logging
	//and output it to the user running fresher. This is so the user can see any output
	//from running the binary to diagnose issues.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalln(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalln(err)
	}

	//Run the command/binary.
	err = cmd.Start()
	if err != nil {
		log.Fatalln(err)
	}

	//Copy output from the command to output from fresher. This way the output from
	//the binary is displayed to the user in real time.
	go io.Copy(os.Stderr, stderr)
	go io.Copy(os.Stdout, stdout)

	//Stop the running binary if it has been rebuilt and will be rerun. This prevents
	//multiple built binaries from running at one time.
	go func() {
		<-stopChan
		cmd.Process.Kill()
	}()
}

// Start calls start() to handle building the running the binary.
func Start() {
	start()

	//Send an event to build and run the binary for the first time when fresher
	//starts. "/" is just a random string to trigger building.
	eventsChan <- fsnotify.Event{
		Name: "/",
		Op:   fsnotify.Write,
	}

	//Block indefintely to continuously watch for file changes and rebuild as needed.
	<-make(chan int)
}
