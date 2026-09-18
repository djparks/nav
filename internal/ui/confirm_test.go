package ui

import (
	"bytes"
	"strings"
	"testing"

	"nav/internal/term"
)

// confirm drives Confirm with a scripted sequence of keypress bytes.
func confirm(t *testing.T, keys string, opts ...func(*ConfirmOptions)) (bool, string) {
	t.Helper()

	var out bytes.Buffer
	o := ConfirmOptions{
		Input:   strings.NewReader(keys),
		Output:  &out,
		Command: "docker ps -a",
		Size:    func() term.Size { return term.Size{Rows: 24, Cols: 80} },
	}
	for _, f := range opts {
		f(&o)
	}

	ok, err := Confirm(o)
	if err != nil {
		t.Fatalf("Confirm returned an error: %v", err)
	}
	return ok, out.String()
}

func TestConfirmYesRuns(t *testing.T) {
	for _, keys := range []string{"y", "Y"} {
		t.Run(keys, func(t *testing.T) {
			if ok, _ := confirm(t, keys); !ok {
				t.Errorf("Confirm(%q) = false, want true", keys)
			}
		})
	}
}

// Everything that is not an explicit yes must decline. Running a command the
// user did not mean to run is the one outcome worth going out of the way to
// avoid, so this is the behaviour most worth pinning down.
func TestConfirmAnythingElseDeclines(t *testing.T) {
	tests := map[string]string{
		"n":            "n",
		"N":            "N",
		"enter":        "\r",
		"esc":          "\x1b",
		"ctrl-c":       "\x03",
		"end of input": "",
	}

	for name, keys := range tests {
		t.Run(name, func(t *testing.T) {
			if ok, _ := confirm(t, keys); ok {
				t.Errorf("Confirm(%q) = true, want false", keys)
			}
		})
	}
}

func TestConfirmIgnoresUnrelatedKeys(t *testing.T) {
	// A stray key is neither answer: the prompt stays up and says so, and
	// the following y still runs the command.
	ok, out := confirm(t, "qy")
	if !ok {
		t.Error("ok = false, want true: the y after a stray key should still count")
	}
	if !strings.Contains(out, "answer y or n") {
		t.Errorf("output does not ask again after a stray key:\n%s", out)
	}
}

func TestConfirmShowsTheCommandAndDescription(t *testing.T) {
	_, out := confirm(t, "\x1b", func(o *ConfirmOptions) {
		o.Command = `docker ps --format "json"`
		o.Description = "List running containers"
	})

	for _, want := range []string{
		"Command:",
		"List running containers",
		`docker ps --format "json"`,
		"[y/N]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestConfirmShowsLongCommandsInFull(t *testing.T) {
	// The user is agreeing to run this, so it is wrapped rather than cut
	// short: hiding part of what will run would be the wrong trade.
	long := "echo " + strings.Repeat("abcdefghij", 12)

	_, out := confirm(t, "\x1b", func(o *ConfirmOptions) {
		o.Command = long
		o.Size = func() term.Size { return term.Size{Rows: 24, Cols: 40} }
	})

	// Every part of the command should appear somewhere in the frame.
	stripped := strings.NewReplacer("\r\n", "", "\x1b[K", "", "  ", "").Replace(out)
	for i := 0; i+10 <= len(long); i += 10 {
		if !strings.Contains(stripped, long[i:i+10]) {
			t.Fatalf("the command was truncated: %q is missing", long[i:i+10])
		}
	}
	// No ellipsis check here: the footer hint is truncated to fit a narrow
	// terminal, so an ellipsis in the frame says nothing about the command.
	// Finding every chunk above is what proves it was wrapped, not cut.
}

func TestConfirmRendersWithinTheTerminal(t *testing.T) {
	_, out := confirm(t, "\x1b", func(o *ConfirmOptions) {
		o.Size = func() term.Size { return term.Size{Rows: 24, Cols: 80} }
	})

	// Raw mode needs every newline paired with a carriage return.
	for i, r := range out {
		if r == '\n' && (i == 0 || out[i-1] != '\r') {
			t.Fatalf("found a \\n not preceded by \\r at byte %d", i)
		}
	}
}

func TestConfirmRequiresInputAndOutput(t *testing.T) {
	if _, err := Confirm(ConfirmOptions{Output: &bytes.Buffer{}}); err == nil {
		t.Error("Confirm with no Input succeeded, want an error")
	}
	if _, err := Confirm(ConfirmOptions{Input: strings.NewReader("")}); err == nil {
		t.Error("Confirm with no Output succeeded, want an error")
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  []string
	}{
		{"short", 10, []string{"short"}},
		{"exactly-10", 10, []string{"exactly-10"}},
		{"elevenchars", 10, []string{"elevenchar", "s"}},
		{"abcdefghij" + "klmnopqrst", 10, []string{"abcdefghij", "klmnopqrst"}},
		{"", 10, []string{""}},
		{"anything", 0, []string{""}},
		// Multi-byte runes are counted, not split.
		{"ééééé", 2, []string{"éé", "éé", "é"}},
	}

	for _, tt := range tests {
		got := wrap(tt.text, tt.width)
		if len(got) != len(tt.want) {
			t.Errorf("wrap(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("wrap(%q, %d)[%d] = %q, want %q", tt.text, tt.width, i, got[i], tt.want[i])
			}
		}
	}
}
