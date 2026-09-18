package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestShellPrefersSHELL(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := Shell(); got != "/usr/bin/fish" {
		t.Errorf("Shell() = %q, want the value of $SHELL", got)
	}
}

func TestShellFallsBack(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := Shell(); got != DefaultShell {
		t.Errorf("Shell() = %q, want %q", got, DefaultShell)
	}
}

func TestRunCapturesStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status, err := Run("echo hello", Options{Shell: "/bin/sh", Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0 (stderr: %s)", status, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}
}

func TestRunCapturesStderrSeparately(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if _, err := Run("echo out; echo err >&2", Options{Shell: "/bin/sh", Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "out" {
		t.Errorf("stdout = %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(stderr.String()); got != "err" {
		t.Errorf("stderr = %q, want %q", got, "err")
	}
}

// Shell syntax is the whole reason commands go through a shell rather than
// being split into arguments, so it is worth pinning down.
func TestRunUsesShellSyntax(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"a pipe", "echo one two three | tr ' ' '-'", "one-two-three"},
		{"quoting", `echo "a  b"`, "a  b"},
		{"variable expansion", "x=5; echo $x", "5"},
		{"and-and", "true && echo yes", "yes"},
		{"command substitution", "echo $(echo nested)", "nested"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if _, err := Run(tt.command, Options{Shell: "/bin/sh", Stdout: &stdout, Stderr: &stdout}); err != nil {
				t.Fatalf("Run returned an error: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunReportsExitStatus(t *testing.T) {
	for _, want := range []int{0, 1, 2, 42} {
		var out bytes.Buffer
		status, err := Run("exit "+strconv.Itoa(want), Options{Shell: "/bin/sh", Stdout: &out, Stderr: &out})
		if err != nil {
			t.Fatalf("Run returned an error: %v", err)
		}
		if status != want {
			t.Errorf("status = %d, want %d", status, want)
		}
	}
}

// A command that fails is not an error on nav's part: it ran and reported
// itself. Run must say so through the status, not through err, or the caller
// would print a second complaint on top of the command's own.
func TestRunFailingCommandIsNotAnError(t *testing.T) {
	var out bytes.Buffer
	status, err := Run("echo nope >&2; exit 3", Options{Shell: "/bin/sh", Stdout: &out, Stderr: &out})
	if err != nil {
		t.Errorf("err = %v, want nil for a command that ran and failed", err)
	}
	if status != 3 {
		t.Errorf("status = %d, want 3", status)
	}
}

func TestRunSignalledCommandUsesShellConvention(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signals work differently on windows")
	}

	var out bytes.Buffer
	// The shell kills itself with SIGINT, which a shell reports as 130.
	status, err := Run("kill -INT $$", Options{Shell: "/bin/sh", Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if status != 130 {
		t.Errorf("status = %d, want 130 (128 + SIGINT)", status)
	}
}

func TestRunMissingShellIsAnError(t *testing.T) {
	var out bytes.Buffer
	_, err := Run("echo hi", Options{
		Shell:  filepath.Join(t.TempDir(), "no-such-shell"),
		Stdout: &out,
		Stderr: &out,
	})
	if err == nil {
		t.Fatal("Run succeeded, want an error for a missing shell")
	}
	if !strings.Contains(err.Error(), "could not run") {
		t.Errorf("error = %v, want it to explain that the shell could not be run", err)
	}
}

func TestRunDefaultsToShellFromEnvironment(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	var out bytes.Buffer
	// Shell is left unset in Options, so Run consults $SHELL.
	status, err := Run("echo from-default", Options{Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
	if got := strings.TrimSpace(out.String()); got != "from-default" {
		t.Errorf("output = %q, want %q", got, "from-default")
	}
}

func TestRunUsesStdin(t *testing.T) {
	var out bytes.Buffer
	status, err := Run("cat", Options{
		Shell:  "/bin/sh",
		Stdin:  strings.NewReader("piped in"),
		Stdout: &out,
		Stderr: &out,
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
	if got := out.String(); got != "piped in" {
		t.Errorf("output = %q, want %q", got, "piped in")
	}
}

func TestRunInDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if _, err := Run("ls", Options{Shell: "/bin/sh", Dir: dir, Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !strings.Contains(out.String(), "marker.txt") {
		t.Errorf("output = %q, want it to list the temp directory", out.String())
	}
}

func TestRunWithCustomEnvironment(t *testing.T) {
	var out bytes.Buffer
	if _, err := Run("echo $NAV_TEST_VAR", Options{
		Shell:  "/bin/sh",
		Env:    []string{"NAV_TEST_VAR=visible"},
		Stdout: &out,
		Stderr: &out,
	}); err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "visible" {
		t.Errorf("output = %q, want %q", got, "visible")
	}
}

func TestExitStatusOfNilState(t *testing.T) {
	if got := exitStatus(nil); got != 0 {
		t.Errorf("exitStatus(nil) = %d, want 0", got)
	}
}

func TestInterruptSignalsAreListed(t *testing.T) {
	if len(interruptSignals()) == 0 {
		t.Error("interruptSignals() is empty, so nav would die with the command it runs")
	}
}
