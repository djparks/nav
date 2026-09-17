package cheat

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Extension is the file suffix nav recognises as a cheatsheet.
const Extension = ".cheat"

// LoadDir reads every *.cheat file under dir, recursively, and returns all
// the cheats it found.
//
// Files are visited in a stable order so the resulting slice is deterministic.
// Parse problems do not abort the walk: everything that parsed is returned
// along with a ParseErrors describing the rest.
func LoadDir(dir string) ([]Cheat, error) {
	paths, err := findCheatFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no %s files found in %s", Extension, dir)
	}

	var (
		cheats []Cheat
		errs   ParseErrors
	)
	for _, path := range paths {
		found, err := LoadFile(path)
		cheats = append(cheats, found...)
		if err != nil {
			var pe ParseErrors
			if errors.As(err, &pe) {
				errs = append(errs, pe...)
				continue
			}
			return cheats, err
		}
	}
	if len(errs) > 0 {
		return cheats, errs
	}
	return cheats, nil
}

// LoadFile reads a single cheatsheet file.
func LoadFile(path string) ([]Cheat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return Parse(f, path)
}

// findCheatFiles walks dir and returns the sorted paths of all cheat files.
func findCheatFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("cheat directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cheat path %s is not a directory", dir)
	}

	var paths []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip hidden directories such as .git.
			if path != dir && len(d.Name()) > 1 && d.Name()[0] == '.' {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) == Extension {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}
