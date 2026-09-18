// Package config decides where nav reads its cheatsheets from.
//
// Three things can have an opinion, and they are consulted in this order:
//
//  1. the NAV_PATH environment variable
//  2. a "path" entry in the configuration file
//  3. a built-in default
//
// Command-line flags beat all of them, but that is the cli package's job:
// this package knows nothing about flags.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment variables nav reads.
const (
	// EnvPath names the cheatsheet directory, overriding the config file.
	EnvPath = "NAV_PATH"
	// EnvConfig names the configuration file, overriding its usual location.
	EnvConfig = "NAV_CONFIG"
)

// Names used to build the usual file locations.
const (
	// AppDir is nav's own directory inside the user's config and data
	// directories.
	AppDir = "nav"
	// FileName is the configuration file's name inside AppDir.
	FileName = "config"
	// CheatDirName is the directory cheatsheets live in, both in the user's
	// data directory and in a checkout of nav.
	CheatDirName = "cheats"
)

// Config is the resolved configuration.
type Config struct {
	// Path is the directory to load .cheat files from. It is never empty:
	// when nothing sets it, it is the built-in default.
	Path string
	// PathSource says where Path came from, in a form fit for an error
	// message, such as `$NAV_PATH` or `/home/u/.config/nav/config:2`. It
	// exists so that "no cheats in <dir>" can also say who chose <dir>.
	PathSource string
	// File is the configuration file that was read, or "" when there was
	// none. A missing file is normal, not an error.
	File string
}

// Options lets a caller — in practice a test — supply its own environment and
// home directory instead of the real ones.
//
// The zero value means "use the real thing", so production code can call
// Load(Options{}).
type Options struct {
	// Getenv reads an environment variable. Defaults to os.Getenv.
	Getenv func(string) string
	// Home is the user's home directory. Defaults to os.UserHomeDir.
	Home string
	// WorkDir is the directory whose "cheats" subdirectory counts as a
	// default. Defaults to ".".
	WorkDir string
}

func (o Options) getenv(name string) string {
	if o.Getenv != nil {
		return o.Getenv(name)
	}
	return os.Getenv(name)
}

func (o Options) home() string {
	if o.Home != "" {
		return o.Home
	}
	// A user with no home directory is unusual but not worth failing over:
	// every use of it is a fallback that something else has already had a
	// chance to override.
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func (o Options) workDir() string {
	if o.WorkDir != "" {
		return o.WorkDir
	}
	return "."
}

// Load resolves the configuration.
//
// A missing configuration file is not an error. A file that exists but cannot
// be read or understood is, because the user clearly meant it to be used.
func Load(opts Options) (Config, error) {
	cfg := Config{}

	file := ConfigFile(opts)
	if contents, err := os.ReadFile(file); err == nil {
		cfg.File = file
		fileCfg, err := parse(string(contents), file, opts)
		if err != nil {
			return Config{}, err
		}
		cfg.Path, cfg.PathSource = fileCfg.Path, fileCfg.PathSource
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("reading %s: %w", file, err)
	}

	// The environment variable wins over the file: it is the more immediate
	// of the two, and is how a one-off override is normally expressed.
	if path := strings.TrimSpace(opts.getenv(EnvPath)); path != "" {
		cfg.Path = expand(path, opts)
		cfg.PathSource = "$" + EnvPath
	}

	if cfg.Path == "" {
		cfg.Path = DefaultPath(opts)
		cfg.PathSource = "the built-in default"
	}
	return cfg, nil
}

// ConfigFile returns the configuration file nav reads: $NAV_CONFIG when set,
// and otherwise <config dir>/nav/config.
func ConfigFile(opts Options) string {
	if file := strings.TrimSpace(opts.getenv(EnvConfig)); file != "" {
		return expand(file, opts)
	}
	return filepath.Join(configHome(opts), AppDir, FileName)
}

// DefaultPath returns the cheatsheet directory to use when neither the
// environment nor the configuration file names one.
//
// A "cheats" directory next to the user wins, so that running nav inside a
// checkout of nav — or inside any project that keeps its own cheats — works
// with no setup at all. Otherwise it is the per-user directory.
func DefaultPath(opts Options) string {
	local := filepath.Join(opts.workDir(), CheatDirName)
	if isDir(local) {
		return local
	}
	return UserPath(opts)
}

// UserPath returns the per-user cheatsheet directory,
// <data dir>/nav/cheats.
func UserPath(opts Options) string {
	return filepath.Join(dataHome(opts), AppDir, CheatDirName)
}

// configHome is $XDG_CONFIG_HOME, or ~/.config.
//
// nav uses the XDG layout on every platform, macOS included. It is where a
// command-line tool's configuration is looked for by habit, and one location
// is easier to document than one per operating system.
func configHome(opts Options) string {
	if dir := strings.TrimSpace(opts.getenv("XDG_CONFIG_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(opts.home(), ".config")
}

// dataHome is $XDG_DATA_HOME, or ~/.local/share.
func dataHome(opts Options) string {
	if dir := strings.TrimSpace(opts.getenv("XDG_DATA_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(opts.home(), ".local", "share")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// expand turns a leading ~ into the user's home directory. Nothing else is
// expanded: a config file is not a shell script, and a value that looks like
// $HOME is far more likely to be a mistake than an instruction.
func expand(path string, opts Options) string {
	home := opts.home()
	if home == "" {
		return path
	}
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	}
	return path
}
