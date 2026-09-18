package config

import (
	"fmt"
	"strings"
)

// The configuration file is a list of "key = value" lines:
//
//	# Where my cheatsheets live.
//	path = ~/cheats
//
// Blank lines are ignored, and so is anything after a '#'. There are no
// sections, no nesting and no quoting: a value is the rest of the line with
// the surrounding spaces removed. That is enough for the handful of settings
// nav has, and it means the file can be read at a glance.

// Keys the configuration file understands.
const (
	// KeyPath is the cheatsheet directory, the equivalent of --path.
	KeyPath = "path"
)

// parse reads the configuration file's contents. file names the file and is
// used only in error messages.
func parse(contents, file string, opts Options) (Config, error) {
	cfg := Config{File: file}

	for i, raw := range strings.Split(contents, "\n") {
		line := i + 1

		text := strings.TrimSpace(strip(raw))
		if text == "" {
			continue
		}

		key, value, found := strings.Cut(text, "=")
		if !found {
			return Config{}, fmt.Errorf("%s:%d: expected 'key = value', got %q", file, line, text)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if value == "" {
			return Config{}, fmt.Errorf("%s:%d: %s has no value", file, line, key)
		}

		switch key {
		case KeyPath:
			cfg.Path = expand(value, opts)
			cfg.PathSource = fmt.Sprintf("%s:%d", file, line)
		default:
			return Config{}, fmt.Errorf("%s:%d: unknown setting %q (known settings: %s)",
				file, line, key, strings.Join(knownKeys(), ", "))
		}
	}
	return cfg, nil
}

// strip removes a trailing comment. A '#' always starts one, wherever it
// appears, so a value cannot contain one — no path worth configuring does.
func strip(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		return line[:i]
	}
	return line
}

// knownKeys lists the settings the file understands, for the error message a
// typo produces.
func knownKeys() []string {
	return []string{KeyPath}
}
