package cheat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCheats builds a temporary cheat directory from a path -> content map.
func writeCheats(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadDirMultipleFiles(t *testing.T) {
	dir := writeCheats(t, map[string]string{
		"a.cheat":         "% alpha\n# first\nls\n",
		"b.cheat":         "% beta\n# second\npwd\n",
		"nested/c.cheat":  "% gamma\n# third\nwhoami\n",
		"notes.txt":       "this is not a cheatsheet\n",
		".hidden/d.cheat": "% delta\n# hidden\nid\n",
	})

	cheats, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned an error: %v", err)
	}
	// a.cheat, b.cheat and nested/c.cheat; notes.txt and .hidden are skipped.
	if len(cheats) != 3 {
		t.Fatalf("got %d cheats, want 3: %v", len(cheats), cheats)
	}

	// Files are walked in sorted path order: a.cheat, b.cheat, nested/c.cheat.
	want := []string{"ls", "pwd", "whoami"}
	for i, cmd := range want {
		if cheats[i].Command != cmd {
			t.Errorf("cheats[%d].Command = %q, want %q", i, cheats[i].Command, cmd)
		}
	}
}

func TestLoadDirRecordsSourcePath(t *testing.T) {
	dir := writeCheats(t, map[string]string{"a.cheat": "% alpha\n# first\nls\n"})

	cheats, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned an error: %v", err)
	}
	if want := filepath.Join(dir, "a.cheat"); cheats[0].Source != want {
		t.Errorf("Source = %q, want %q", cheats[0].Source, want)
	}
}

func TestLoadDirCollectsErrorsAcrossFiles(t *testing.T) {
	dir := writeCheats(t, map[string]string{
		"good.cheat": "% alpha\n# first\nls\n",
		"bad.cheat":  "ls -l\n",
	})

	cheats, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir succeeded, want an error from bad.cheat")
	}

	var errs ParseErrors
	if !errors.As(err, &errs) {
		t.Fatalf("error is %T, want ParseErrors", err)
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), err)
	}
	if !strings.HasSuffix(errs[0].Source, "bad.cheat") {
		t.Errorf("error source = %q, want it to end in bad.cheat", errs[0].Source)
	}
	// The valid file is still usable.
	if len(cheats) != 1 || cheats[0].Command != "ls" {
		t.Errorf("got cheats %v, want just \"ls\"", cheats)
	}
}

func TestLoadDirMissingDirectory(t *testing.T) {
	_, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("LoadDir succeeded, want an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}

func TestLoadDirNotADirectory(t *testing.T) {
	dir := writeCheats(t, map[string]string{"a.cheat": "% alpha\n# first\nls\n"})

	_, err := LoadDir(filepath.Join(dir, "a.cheat"))
	if err == nil {
		t.Fatal("LoadDir succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %v, want it to mention that the path is not a directory", err)
	}
}

func TestLoadDirNoCheatFiles(t *testing.T) {
	dir := writeCheats(t, map[string]string{"notes.txt": "nothing here\n"})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no .cheat files found") {
		t.Errorf("error = %v, want it to mention that no cheat files were found", err)
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.cheat"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}

// The cheats shipped with the repository must parse cleanly.
func TestBundledCheatsAreValid(t *testing.T) {
	dir := filepath.Join("..", "..", "cheats")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no bundled cheats directory: %v", err)
	}

	cheats, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("bundled cheats do not parse: %v", err)
	}
	if len(cheats) == 0 {
		t.Fatal("bundled cheats directory produced no cheats")
	}
}
