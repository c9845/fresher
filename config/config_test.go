package config

import "testing"

func TestValidate(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Make sure a known good config passes validation.
	err := cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}

	//Edit the config to break things.
	cfg.WorkingDir = ""
	err = cfg.validate()
	if err == nil {
		t.Fatal("Error about bad WorkingDir should have been returned.")
		return
	}
	cfg.WorkingDir = "."

	//Edit the config to break things.
	cfg.EntryPoint = ""
	err = cfg.validate()
	if err == nil {
		t.Fatal("Error about bad EntryPoint should have been returned.")
		return
	}
	cfg.EntryPoint = "."

	//Test defaults being set.
	cfg.TempDir = ""
	err = cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}
	if cfg.TempDir != newDefaultConfig().TempDir {
		t.Fatal("Default value not set for TempDir.")
		return
	}

	cfg.ExtensionsToWatch = []string{}
	err = cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}
	if len(cfg.ExtensionsToWatch) != len(newDefaultConfig().ExtensionsToWatch) {
		t.Fatal("Default value not set for ExtensionsToWatch.", cfg.ExtensionsToWatch, newDefaultConfig().ExtensionsToWatch)
		return
	}

	cfg.BuildDelayMilliseconds = -100
	err = cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}
	if cfg.BuildDelayMilliseconds != newDefaultConfig().BuildDelayMilliseconds {
		t.Fatal("Default value not set for BuildDelayMilliseconds.", cfg.BuildDelayMilliseconds, newDefaultConfig().BuildDelayMilliseconds)
		return
	}

	cfg.BuildName = ""
	err = cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}
	if cfg.BuildName != newDefaultConfig().BuildName {
		t.Fatal("Default value not set for BuildName.")
		return
	}

	cfg.BuildLogFilename = ""
	err = cfg.validate()
	if err != nil {
		t.Fatal(err)
		return
	}
	if cfg.BuildLogFilename != newDefaultConfig().BuildLogFilename {
		t.Fatal("Default value not set for BuildLogFilename.")
		return
	}
}

func TestIsTempDir(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Test with known matching dir.
	p := "./tmp"
	yes, err := cfg.IsTempDir(p)
	if err != nil {
		t.Fatal(err)
		return
	}
	if !yes {
		t.Fatal("TempDir path matches, IsTempDir should have returned true.")
		return
	}

	//Test with known non-matching dir.
	p = "./temporary"
	yes, err = cfg.IsTempDir(p)
	if err != nil {
		t.Fatal(err)
		return
	}
	if yes {
		t.Fatal("TempDir path does not match, IsTempDir should have returned false.")
		return
	}
}

func TestIsDirectoryToIgnore(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Test with known matching dir.
	p := "node_modules"
	yes := cfg.IsDirectoryToIgnore(p)
	if !yes {
		t.Fatal("DirectoryiesToIgnore path matches, IsDirectoryToIgnore should have returned true.")
		return
	}

	//Test with known non-matching dir.
	p = "some_source_dir"
	yes = cfg.IsDirectoryToIgnore(p)
	if yes {
		t.Fatal("DirectoryiesToIgnore path matches, IsDirectoryToIgnore should have returned true.")
		return
	}
}

func TestIsRebuildExtension(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Test with known rebuild extension.
	ext := ".go"
	yes := cfg.IsRebuildExtension(ext)
	if !yes {
		t.Fatal("IsRebuildExtension should have returned true.", cfg.NoRebuildExtensions)
		return
	}

	//Test with known no-rebuild extension.
	ext = ".html"
	yes = cfg.IsRebuildExtension(ext)
	if yes {
		t.Fatal("IsRebuildExtension should have returned false.", cfg.NoRebuildExtensions)
		return
	}
}

func TestIsExtensionToWatch(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Test with known rebuild extension.
	ext := ".go"
	yes := cfg.IsExtensionToWatch(ext)
	if !yes {
		t.Fatal("IsExtensionToWatch should have returned true.", cfg.NoRebuildExtensions)
		return
	}

	//Test with known not-watched extension.
	ext = ".css"
	yes = cfg.IsExtensionToWatch(ext)
	if yes {
		t.Fatal("IsExtensionToWatch should have returned false.", cfg.NoRebuildExtensions)
		return
	}
}

func TestUsingDefaults(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	if !cfg.UsingDefaults() {
		t.Fatal("UsingDefaults should have returned true.")
		return
	}
}

func TestOverrideTags(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()
	oldTags := cfg.GoTags

	//Modify tags.
	newTags := "newtag newtag2"
	cfg.OverrideTags(newTags)
	if cfg.GoTags != newTags {
		t.Fatal("Tags not overridden correctly.")
		return
	}

	//Make sure old tags don't exist.
	if cfg.GoTags == oldTags {
		t.Fatal("Old tags still exist, this should not be!")
		return
	}
}

func TestOverrideVerbose(t *testing.T) {
	//Get a default config to work from.
	cfg := newDefaultConfig()

	//Modify
	cfg.OverrideVerbose(true)
	if cfg.Verbose != true {
		t.Fatal("Verbose not overridden correctly.")
		return
	}
}
