package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exec runs nav with the given args and returns the exit code, stdout and stderr.
func exec(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// cheatDir creates a temporary cheat directory from a name -> content map.
func cheatDir(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h", "-help"} {
		t.Run(flag, func(t *testing.T) {
			code, stdout, stderr := exec(t, flag)
			if code != ExitOK {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
			}
			for _, want := range []string{"Usage:", "--version", "--path", "--list"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("help output is missing %q:\n%s", want, stdout)
				}
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want it empty", stderr)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		t.Run(flag, func(t *testing.T) {
			code, stdout, stderr := exec(t, flag)
			if code != ExitOK {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
			}
			if want := "nav " + Version + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	code, _, stderr := exec(t, "--nope")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "nav --help") {
		t.Errorf("stderr does not point at --help:\n%s", stderr)
	}
}

func TestUnexpectedArgumentIsAUsageError(t *testing.T) {
	code, _, stderr := exec(t, "extra")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, `unexpected argument "extra"`) {
		t.Errorf("stderr = %q, want it to name the unexpected argument", stderr)
	}
}

func TestListPrintsCheats(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"git.cheat": "% git, branch\n# Show the current branch\ngit rev-parse --abbrev-ref HEAD\n",
	})

	code, stdout, stderr := exec(t, "--path", dir, "--list")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	for _, want := range []string{"git,branch: Show the current branch", "git rev-parse --abbrev-ref HEAD"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout is missing %q:\n%s", want, stdout)
		}
	}
}

func TestDefaultRunReportsWhatWasLoaded(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"a.cheat": "% shell\n# list files\nls\n# print the working directory\npwd\n",
	})

	code, stdout, stderr := exec(t, "--path", dir)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "Loaded 2 cheat(s)") {
		t.Errorf("stdout = %q, want it to report 2 cheats", stdout)
	}
}

func TestMissingCheatDirectoryIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	code, _, stderr := exec(t, "--path", missing)
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "cheat directory") {
		t.Errorf("stderr = %q, want it to mention the cheat directory", stderr)
	}
}

func TestParseErrorsAreReportedButValidCheatsStillLoad(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"good.cheat": "% shell\n# list files\nls\n",
		"bad.cheat":  "oops-no-tags\n",
	})

	code, stdout, stderr := exec(t, "--path", dir, "--list")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "bad.cheat:1") {
		t.Errorf("stderr = %q, want it to name bad.cheat line 1", stderr)
	}
	if !strings.Contains(stdout, "ls") {
		t.Errorf("stdout = %q, want the valid cheat from good.cheat", stdout)
	}
}

func TestAllCheatsUnusableIsAnError(t *testing.T) {
	dir := cheatDir(t, map[string]string{"bad.cheat": "oops-no-tags\n"})

	code, _, stderr := exec(t, "--path", dir)
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "no usable cheats") {
		t.Errorf("stderr = %q, want it to say there are no usable cheats", stderr)
	}
}
