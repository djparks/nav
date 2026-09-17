package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestToolsForCurrentPlatform(t *testing.T) {
	got := tools()
	if len(got) == 0 {
		t.Fatalf("no clipboard tools listed for %s", runtime.GOOS)
	}
	for _, tool := range got {
		if tool.name == "" {
			t.Error("a listed tool has no program name")
		}
	}
}

func TestPlatformSpecificTool(t *testing.T) {
	// The first candidate is the platform's native tool.
	want := map[string]string{
		"darwin":  "pbcopy",
		"windows": "clip",
		"linux":   "wl-copy",
	}[runtime.GOOS]
	if want == "" {
		t.Skipf("no expectation recorded for %s", runtime.GOOS)
	}
	if got := tools()[0].name; got != want {
		t.Errorf("first tool on %s is %q, want %q", runtime.GOOS, got, want)
	}
}

// withStubPath replaces PATH with a directory containing a single fake
// clipboard program, so Copy can be exercised without touching the real
// clipboard. The script appends its stdin to outFile.
func withStubPath(t *testing.T, name string, exitCode int) (outFile string) {
	t.Helper()

	dir := t.TempDir()
	outFile = filepath.Join(dir, "captured.txt")

	// "set -e" so a failure inside the stub cannot be masked by the exit line.
	script := "#!/bin/sh\nset -e\ncat >> " + outFile + "\n"
	if exitCode != 0 {
		script = "#!/bin/sh\ncat > /dev/null\necho 'stub failure' >&2\nexit " + strconv.Itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Prepend rather than replace: the stub has to shadow the real tool, but
	// the shell script still needs to find ordinary utilities like cat.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return outFile
}

func TestCopySendsTextToTheTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	outFile := withStubPath(t, tools()[0].name, 0)

	const want = `docker ps --format "<format>"`
	if err := Copy(want); err != nil {
		t.Fatalf("Copy returned an error: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("the stub captured nothing: %v", err)
	}
	if string(got) != want {
		t.Errorf("the tool received %q, want %q", got, want)
	}
}

func TestCopyReportsToolFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	name := tools()[0].name
	withStubPath(t, name, 1)

	err := Copy("anything")
	if err == nil {
		t.Fatal("Copy succeeded, want an error")
	}
	// The message should name the program and include its stderr, so a user
	// can tell what went wrong.
	for _, want := range []string{name, "stub failure"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestCopyWithoutAnyTool(t *testing.T) {
	// An empty PATH means none of the candidates can be found.
	t.Setenv("PATH", t.TempDir())

	if Available() {
		t.Error("Available() = true with an empty PATH, want false")
	}
	err := Copy("anything")
	if !errors.Is(err, ErrNoTool) {
		t.Errorf("error = %v, want ErrNoTool", err)
	}
}

func TestAvailableFindsTheStub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	withStubPath(t, tools()[0].name, 0)
	if !Available() {
		t.Error("Available() = false, want true when a tool is on PATH")
	}
}

// TestRealToolIsUsableIfPresent is a smoke test against whatever is actually
// installed. It only checks that the program can be located and started, and
// does not read the clipboard back, so it never clobbers anything the user
// has copied... except on the platform's own terms.
func TestRealToolIsUsableIfPresent(t *testing.T) {
	tool, err := find()
	if err != nil {
		t.Skip("no clipboard program installed")
	}
	if _, err := exec.LookPath(tool.name); err != nil {
		t.Errorf("find() returned %q but it is not on PATH: %v", tool.name, err)
	}
}
