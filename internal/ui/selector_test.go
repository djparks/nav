package ui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"nav/internal/cheat"
	"nav/internal/term"
)

var testCheats = []cheat.Cheat{
	{Tags: []string{"git", "branch"}, Description: "Show the current branch", Command: "git rev-parse --abbrev-ref HEAD"},
	{Tags: []string{"docker"}, Description: "List running containers", Command: "docker ps"},
	{Tags: []string{"docker"}, Description: "List images", Command: "docker images"},
	{Tags: []string{"kubernetes"}, Description: "List pods", Command: "kubectl get pods"},
}

// run drives the selector with a scripted sequence of keypress bytes.
// Reaching the end of the script counts as quitting, so a script that does
// not press Enter always terminates.
func run(t *testing.T, keys string, opts ...func(*Options)) (Result, string, error) {
	t.Helper()

	var out bytes.Buffer
	o := Options{
		Input:  strings.NewReader(keys),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 24, Cols: 80} },
	}
	for _, f := range opts {
		f(&o)
	}

	result, err := Run(testCheats, o)
	return result, out.String(), err
}

func TestSelectFirstMatchWithEnter(t *testing.T) {
	result, _, err := run(t, "\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !result.Selected {
		t.Fatal("Selected = false, want true")
	}
	if want := testCheats[0].Command; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestTypingFiltersTheList(t *testing.T) {
	// "kubectl" appears only in the fourth cheat's command, so typing it
	// narrows the list to that one and Enter selects it.
	result, _, err := run(t, "kubectl\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !result.Selected {
		t.Fatal("Selected = false, want true")
	}
	if want := "kubectl get pods"; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestArrowKeysMoveTheHighlight(t *testing.T) {
	// Filter to the two docker cheats, move down once, then select.
	result, _, err := run(t, "docker\x1b[B\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := "docker images"; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestHighlightStopsAtTheEnds(t *testing.T) {
	// Up at the top of the list stays on the first entry.
	result, _, err := run(t, "docker\x1b[A\x1b[A\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := "docker ps"; result.Cheat.Command != want {
		t.Errorf("after moving up at the top, Command = %q, want %q", result.Cheat.Command, want)
	}

	// Down past the bottom stays on the last entry.
	result, _, err = run(t, "docker\x1b[B\x1b[B\x1b[B\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := "docker images"; result.Cheat.Command != want {
		t.Errorf("after moving down past the end, Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestBackspaceWidensTheSearch(t *testing.T) {
	// "dockerx" matches nothing; a backspace brings back the docker cheats.
	result, out, err := run(t, "dockerx\x7f\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !result.Selected {
		t.Fatalf("Selected = false, want true. Output:\n%s", out)
	}
	if want := "docker ps"; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestClearQueryAndDeleteWord(t *testing.T) {
	// Ctrl-U empties the search box, so every cheat matches again.
	result, _, err := run(t, "kubectl\x15\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if result.Cheat.Command != testCheats[0].Command {
		t.Errorf("after ctrl-U, Command = %q, want the first cheat", result.Cheat.Command)
	}

	// Ctrl-W drops "images", leaving "docker" and both docker cheats.
	result, _, err = run(t, "docker images\x17\x1b[B\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := "docker images"; result.Cheat.Command != want {
		t.Errorf("after ctrl-W, Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestEnterWithNoMatchesDoesNotSelect(t *testing.T) {
	result, out, err := run(t, "zzzznotfound\r")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if result.Selected {
		t.Error("Selected = true, want false when nothing matches")
	}
	if !strings.Contains(out, "no cheats match") {
		t.Errorf("output does not tell the user nothing matched:\n%s", out)
	}
}

func TestQuitKeys(t *testing.T) {
	for name, keys := range map[string]string{
		"esc":    "\x1b",
		"ctrl-c": "\x03",
	} {
		t.Run(name, func(t *testing.T) {
			result, _, err := run(t, keys)
			if err != nil {
				t.Fatalf("Run returned an error: %v", err)
			}
			if result.Selected {
				t.Error("Selected = true, want false after quitting")
			}
		})
	}
}

func TestEndOfInputQuits(t *testing.T) {
	// An empty script means the input closed immediately, which must be
	// treated as quitting rather than as an error.
	result, _, err := run(t, "")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if result.Selected {
		t.Error("Selected = true, want false")
	}
}

func TestInitialQueryIsApplied(t *testing.T) {
	result, _, err := run(t, "\r", func(o *Options) { o.Query = "kubectl" })
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := "kubectl get pods"; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestCopyKeyCopiesTheHighlightedCommand(t *testing.T) {
	var copied []string
	result, out, err := run(t, "docker\x1b[B\x19\x1b", func(o *Options) {
		o.Copy = func(s string) error {
			copied = append(copied, s)
			return nil
		}
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if len(copied) != 1 {
		t.Fatalf("Copy was called %d times, want 1", len(copied))
	}
	// The highlight was moved down once, so the second docker cheat is copied.
	if want := "docker images"; copied[0] != want {
		t.Errorf("copied %q, want %q", copied[0], want)
	}
	if !result.Copied {
		t.Error("Result.Copied = false, want true")
	}
	if !strings.Contains(out, "copied to clipboard") {
		t.Errorf("output does not confirm the copy:\n%s", out)
	}
}

func TestCopyKeepsTheSelectorOpen(t *testing.T) {
	// Ctrl-Y copies but does not exit, so the following Enter still selects.
	result, _, err := run(t, "kubectl\x19\r", func(o *Options) {
		o.Copy = func(string) error { return nil }
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !result.Selected {
		t.Error("Selected = false, want true: ctrl-Y should not close the selector")
	}
	if !result.Copied {
		t.Error("Copied = false, want true")
	}
}

func TestCopyFailureIsReportedNotFatal(t *testing.T) {
	result, out, err := run(t, "kubectl\x19\r", func(o *Options) {
		o.Copy = func(string) error { return errors.New("pbcopy exploded") }
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !strings.Contains(out, "pbcopy exploded") {
		t.Errorf("output does not mention the copy failure:\n%s", out)
	}
	if result.Copied {
		t.Error("Copied = true, want false after a failed copy")
	}
	if !result.Selected {
		t.Error("Selected = false: a failed copy should not stop the user selecting")
	}
}

func TestCopyWithoutClipboardSupport(t *testing.T) {
	// Copy is nil, meaning no clipboard tool was found.
	_, out, err := run(t, "\x19\x1b")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !strings.Contains(out, "clipboard unavailable") {
		t.Errorf("output does not say the clipboard is unavailable:\n%s", out)
	}
}

func TestCopyWithNoMatches(t *testing.T) {
	_, out, err := run(t, "zzzz\x19\x1b", func(o *Options) {
		o.Copy = func(string) error {
			t.Error("Copy was called even though nothing matched")
			return nil
		}
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !strings.Contains(out, "nothing to copy") {
		t.Errorf("output does not say there is nothing to copy:\n%s", out)
	}
}

func TestRenderShowsMatchesAndHints(t *testing.T) {
	_, out, err := run(t, "docker\x1b")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	for _, want := range []string{
		"Search: ",
		"docker",
		"List running containers",
		"docker ps",
		"^Y copy",
		"esc quit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output is missing %q", want)
		}
	}
}

func TestRenderUsesCarriageReturns(t *testing.T) {
	// In raw mode the terminal does not turn \n into \r\n, so every line
	// break the selector emits must include the \r itself. A bare \n would
	// leave the output stair-stepping across the screen.
	_, out, err := run(t, "\x1b")
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	for i, r := range out {
		if r == '\n' && (i == 0 || out[i-1] != '\r') {
			t.Fatalf("found a \\n not preceded by \\r at byte %d", i)
		}
	}
}

func TestRenderFitsTheTerminalHeight(t *testing.T) {
	// A short terminal must not draw more rows than it has. Ten rows leaves
	// six for the list, which is three cheats at two rows each.
	var out bytes.Buffer
	_, err := Run(testCheats, Options{
		Input:  strings.NewReader("\x1b"),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 10, Cols: 80} },
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	// Count the rows in the first frame.
	frame := out.String()
	rows := strings.Count(frame, "\r\n")
	if rows > 10 {
		t.Errorf("drew %d rows in a 10-row terminal", rows)
	}
	// The fourth cheat does not fit and must not be drawn.
	if strings.Contains(frame, "kubectl get pods") {
		t.Error("drew a cheat that does not fit on screen")
	}
}

func TestRenderTruncatesToTerminalWidth(t *testing.T) {
	long := cheat.Cheat{
		Tags:        []string{"x"},
		Description: strings.Repeat("d", 200),
		Command:     strings.Repeat("c", 200),
	}
	var out bytes.Buffer
	_, err := Run([]cheat.Cheat{long}, Options{
		Input:  strings.NewReader("\x1b"),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 24, Cols: 40} },
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if strings.Contains(out.String(), strings.Repeat("c", 45)) {
		t.Error("a long command was not truncated to the terminal width")
	}
	if !strings.Contains(out.String(), "…") {
		t.Error("truncated text is not marked with an ellipsis")
	}
}

func TestScrollingKeepsTheHighlightVisible(t *testing.T) {
	// Twenty cheats in a terminal with room for three: moving to the last
	// one must scroll the window rather than run off the bottom.
	many := make([]cheat.Cheat, 20)
	for i := range many {
		many[i] = cheat.Cheat{
			Tags:        []string{"t"},
			Description: "cheat",
			Command:     "command-" + string(rune('a'+i)),
		}
	}

	var out bytes.Buffer
	// End jumps to the last entry.
	result, err := Run(many, Options{
		Input:  strings.NewReader("\x1b[F\r"),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 10, Cols: 80} },
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := many[19].Command; result.Cheat.Command != want {
		t.Fatalf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
	// The final frame must contain the highlighted entry.
	frames := strings.Split(out.String(), ansiHome)
	last := frames[len(frames)-1]
	if !strings.Contains(last, many[19].Command) {
		t.Error("the highlighted entry is not visible in the final frame")
	}
}

func TestPageKeysMoveAScreenful(t *testing.T) {
	many := make([]cheat.Cheat, 20)
	for i := range many {
		many[i] = cheat.Cheat{Tags: []string{"t"}, Description: "cheat", Command: "command-" + string(rune('a'+i))}
	}

	var out bytes.Buffer
	// Rows 10 leaves 6 rows, so 3 entries per page. Page down moves 3.
	result, err := Run(many, Options{
		Input:  strings.NewReader("\x1b[6~\r"),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 10, Cols: 80} },
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if want := many[3].Command; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestKeypressSplitAcrossReads(t *testing.T) {
	// A terminal can deliver an escape sequence in pieces. The selector must
	// wait for the rest instead of treating the Esc as a quit.
	r := &chunkedReader{chunks: []string{"docker", "\x1b", "[B", "\r"}}

	var out bytes.Buffer
	result, err := Run(testCheats, Options{
		Input:  r,
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 24, Cols: 80} },
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if !result.Selected {
		t.Fatal("Selected = false: the split escape sequence was treated as a quit")
	}
	if want := "docker images"; result.Cheat.Command != want {
		t.Errorf("Cheat.Command = %q, want %q", result.Cheat.Command, want)
	}
}

func TestRunRequiresInputAndOutput(t *testing.T) {
	if _, err := Run(testCheats, Options{Output: &bytes.Buffer{}}); err == nil {
		t.Error("Run with no Input succeeded, want an error")
	}
	if _, err := Run(testCheats, Options{Input: strings.NewReader("")}); err == nil {
		t.Error("Run with no Output succeeded, want an error")
	}
}

func TestRunWithNoCheats(t *testing.T) {
	var out bytes.Buffer
	result, err := Run(nil, Options{
		Input:  strings.NewReader("\r\x1b"),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if result.Selected {
		t.Error("Selected = true, want false when there are no cheats")
	}
}

func TestRunReportsWriteErrors(t *testing.T) {
	_, err := Run(testCheats, Options{
		Input:  strings.NewReader("\r"),
		Output: failingWriter{},
	})
	if err == nil {
		t.Fatal("Run succeeded, want the write error to be reported")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		// Multi-byte runes must be counted, not their bytes, and never split.
		{"héllo wörld", 6, "héllo…"},
		{"ééé", 3, "ééé"},
	}

	for _, tt := range tests {
		if got := truncate(tt.text, tt.width); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
		}
	}
}

// chunkedReader hands out its chunks one Read at a time, simulating a
// terminal that delivers a key sequence in pieces.
type chunkedReader struct {
	chunks []string
	i      int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.i])
	r.i++
	return n, nil
}

// failingWriter fails every write, standing in for a closed terminal.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("terminal went away") }
