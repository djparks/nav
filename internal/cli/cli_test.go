package cli

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nav/internal/cheat"
	"nav/internal/ui"
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
			for _, want := range []string{"Usage:", "--version", "--path", "--list", "--query", "^Y"} {
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

func TestListHonoursQuery(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"a.cheat": "% shell\n# list files\nls\n# print the working directory\npwd\n",
	})

	code, stdout, stderr := exec(t, "--path", dir, "--list", "--query", "pwd")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "pwd") {
		t.Errorf("stdout = %q, want the matching cheat", stdout)
	}
	if strings.Contains(stdout, "list files") {
		t.Errorf("stdout = %q, want the non-matching cheat filtered out", stdout)
	}
}

func TestQueryShorthand(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"a.cheat": "% shell\n# list files\nls\n# print the working directory\npwd\n",
	})

	code, stdout, _ := exec(t, "--path", dir, "--list", "-q", "pwd")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if strings.Contains(stdout, "list files") {
		t.Errorf("-q did not filter: %q", stdout)
	}
}

func TestListWithNoMatchesIsAnError(t *testing.T) {
	dir := cheatDir(t, map[string]string{"a.cheat": "% shell\n# list files\nls\n"})

	code, _, stderr := exec(t, "--path", dir, "--list", "-q", "nothingmatchesthis")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "no cheats match") {
		t.Errorf("stderr = %q, want it to say nothing matched", stderr)
	}
}

// Without a terminal the selector cannot run, so nav falls back to listing
// the matches. The test suite has no controlling terminal, which is exactly
// the situation being checked.
func TestDefaultRunFallsBackToListingWithoutATerminal(t *testing.T) {
	dir := cheatDir(t, map[string]string{
		"a.cheat": "% shell\n# list files\nls\n# print the working directory\npwd\n",
	})

	code, stdout, stderr := exec(t, "--path", dir, "-q", "pwd")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "pwd") {
		t.Errorf("stdout = %q, want the matching cheat listed", stdout)
	}
	if !strings.Contains(stderr, "no terminal available") {
		t.Errorf("stderr = %q, want it to explain the fallback", stderr)
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

func TestPrintSelected(t *testing.T) {
	var buf bytes.Buffer
	c := cheat.Cheat{
		Tags:        []string{"git", "branch"},
		Description: "Show the current branch",
		Command:     "git rev-parse --abbrev-ref HEAD",
	}
	printSelected(&buf, c, c.Command)

	got := buf.String()
	// The command must be on a line of its own, with no prefix, so the
	// output can be piped into a shell or copied verbatim.
	if !strings.Contains(got, "\ngit rev-parse --abbrev-ref HEAD\n") {
		t.Errorf("the command is not on a bare line of its own:\n%s", got)
	}
	for _, want := range []string{"# Show the current branch", "# tags: git, branch"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
}

func TestPrintSelectedShowsTheFilledCommand(t *testing.T) {
	// printSelected prints the completed command, not the template, so a
	// filled-in value reaches the user rather than the placeholder.
	var buf bytes.Buffer
	printSelected(&buf, cheat.Cheat{
		Tags:        []string{"docker"},
		Description: "List containers",
		Command:     `docker ps --format "<format>"`,
	}, `docker ps --format "json"`)

	got := buf.String()
	if !strings.Contains(got, `docker ps --format "json"`) {
		t.Errorf("output does not contain the filled command:\n%s", got)
	}
	if strings.Contains(got, "<format>") {
		t.Errorf("output still contains the placeholder:\n%s", got)
	}
}

func TestPrintAndYesContradict(t *testing.T) {
	dir := cheatDir(t, map[string]string{"a.cheat": "% shell\n# list files\nls\n"})

	code, _, stderr := exec(t, "--path", dir, "--print", "--yes")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "contradict") {
		t.Errorf("stderr = %q, want it to explain the conflict", stderr)
	}
}

func TestExecuteReportsSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := execute("echo ran-ok", &stdout, &stderr); err != nil {
		t.Fatalf("execute returned an error: %v", err)
	}
	if !strings.Contains(stdout.String(), "ran-ok") {
		t.Errorf("stdout = %q, want the command's output", stdout.String())
	}
	// The command is echoed to stderr, so stdout carries only its output and
	// `nav --yes ... > file` stays useful.
	if !strings.Contains(stderr.String(), "echo ran-ok") {
		t.Errorf("stderr = %q, want the command echoed to it", stderr.String())
	}
	if strings.Contains(stdout.String(), "echo ran-ok") {
		t.Errorf("stdout = %q, want it free of nav's own banner", stdout.String())
	}
}

func TestExecutePropagatesExitStatus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := execute("exit 7", &stdout, &stderr)
	if err == nil {
		t.Fatal("execute succeeded, want a failure carrying the exit status")
	}

	var cf *commandFailed
	if !errors.As(err, &cf) {
		t.Fatalf("error is %T, want *commandFailed", err)
	}
	if cf.status != 7 {
		t.Errorf("status = %d, want 7", cf.status)
	}
}

// A failing command has already explained itself on its own stderr, so nav
// exits with its status and adds nothing of its own.
func TestExitCodeOfAFailedCommandIsItsStatusAndIsQuiet(t *testing.T) {
	var stderr bytes.Buffer
	if got := exitCode(&commandFailed{status: 7}, &stderr); got != 7 {
		t.Errorf("exit code = %d, want 7", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("nav added its own complaint on top of the command's: %q", stderr.String())
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		want      int
		wantNoisy bool
	}{
		{"success", nil, ExitOK, false},
		{"help", flag.ErrHelp, ExitOK, false},
		{"plain error", errors.New("boom"), ExitError, true},
		{"usage error", usagef("bad flag"), ExitUsage, true},
		{"command status 1", &commandFailed{status: 1}, 1, false},
		{"command status 130", &commandFailed{status: 130}, 130, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if got := exitCode(tt.err, &stderr); got != tt.want {
				t.Errorf("exit code = %d, want %d", got, tt.want)
			}
			if noisy := stderr.Len() > 0; noisy != tt.wantNoisy {
				t.Errorf("wrote to stderr = %v, want %v (%q)", noisy, tt.wantNoisy, stderr.String())
			}
		})
	}
}

func TestCommandFailedMessage(t *testing.T) {
	e := &commandFailed{status: 3}
	if !strings.Contains(e.Error(), "3") {
		t.Errorf("Error() = %q, want it to mention the status", e.Error())
	}
}

// finish is where the run-or-print decision is made, and it is the part of
// the flow a terminal is not needed for.
func TestFinish(t *testing.T) {
	selected := ui.Outcome{
		Cheat:    cheat.Cheat{Tags: []string{"demo"}, Description: "Say hi"},
		Command:  "echo hi-there",
		Selected: true,
	}
	confirmed := selected
	confirmed.Run = true

	tests := []struct {
		name        string
		outcome     ui.Outcome
		mode        execMode
		wantRan     bool
		wantPrinted bool
	}{
		{"declined prints the command", selected, askFirst, false, true},
		{"confirmed runs it", confirmed, askFirst, true, false},
		{"--yes runs without a confirmation", selected, runWithoutAsking, true, false},
		{"--print only prints", selected, printOnce, false, true},
		{"nothing selected does neither", ui.Outcome{}, askFirst, false, false},
		{"--yes with nothing selected runs nothing", ui.Outcome{}, runWithoutAsking, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := finish(tt.outcome, tt.mode, &stdout, &stderr); err != nil {
				t.Fatalf("finish returned an error: %v", err)
			}

			ran := strings.Contains(stdout.String(), "hi-there") &&
				!strings.Contains(stdout.String(), "echo hi-there")
			if ran != tt.wantRan {
				t.Errorf("ran = %v, want %v (stdout %q, stderr %q)",
					ran, tt.wantRan, stdout.String(), stderr.String())
			}

			printed := strings.Contains(stdout.String(), "echo hi-there")
			if printed != tt.wantPrinted {
				t.Errorf("printed = %v, want %v (stdout %q)", printed, tt.wantPrinted, stdout.String())
			}
		})
	}
}

func TestFinishReportsACopyWhenQuitting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := finish(ui.Outcome{Copied: true}, askFirst, &stdout, &stderr); err != nil {
		t.Fatalf("finish returned an error: %v", err)
	}
	if !strings.Contains(stderr.String(), "copied to the clipboard") {
		t.Errorf("stderr = %q, want it to confirm the copy", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing printed when no cheat was selected", stdout.String())
	}
}

func TestFinishPropagatesACommandFailure(t *testing.T) {
	outcome := ui.Outcome{Command: "exit 5", Selected: true, Run: true}

	var stdout, stderr bytes.Buffer
	err := finish(outcome, askFirst, &stdout, &stderr)

	var cf *commandFailed
	if !errors.As(err, &cf) {
		t.Fatalf("error is %T (%v), want *commandFailed", err, err)
	}
	if cf.status != 5 {
		t.Errorf("status = %d, want 5", cf.status)
	}
}
