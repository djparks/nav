package cheat

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSingleCheat(t *testing.T) {
	in := `% git, branch

# Show the current branch name
git rev-parse --abbrev-ref HEAD
`
	cheats, err := Parse(strings.NewReader(in), "git.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 1 {
		t.Fatalf("got %d cheats, want 1", len(cheats))
	}

	c := cheats[0]
	if want := []string{"git", "branch"}; !equal(c.Tags, want) {
		t.Errorf("Tags = %v, want %v", c.Tags, want)
	}
	if want := "Show the current branch name"; c.Description != want {
		t.Errorf("Description = %q, want %q", c.Description, want)
	}
	if want := "git rev-parse --abbrev-ref HEAD"; c.Command != want {
		t.Errorf("Command = %q, want %q", c.Command, want)
	}
	if c.Source != "git.cheat" {
		t.Errorf("Source = %q, want %q", c.Source, "git.cheat")
	}
	if c.Line != 4 {
		t.Errorf("Line = %d, want 4", c.Line)
	}
}

func TestParseIgnoresCommentsAndBlankLines(t *testing.T) {
	in := `; a file-level note
;

% shell

; another note

#   Print the working directory
pwd

`
	cheats, err := Parse(strings.NewReader(in), "shell.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 1 {
		t.Fatalf("got %d cheats, want 1", len(cheats))
	}
	if want := "Print the working directory"; cheats[0].Description != want {
		t.Errorf("Description = %q, want %q", cheats[0].Description, want)
	}
	if cheats[0].Command != "pwd" {
		t.Errorf("Command = %q, want %q", cheats[0].Command, "pwd")
	}
}

func TestParseJoinsMultipleDescriptionLines(t *testing.T) {
	in := `% docker
# List running containers
# in the chosen output format
docker ps --format "<format>"
`
	cheats, err := Parse(strings.NewReader(in), "docker.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	want := "List running containers in the chosen output format"
	if cheats[0].Description != want {
		t.Errorf("Description = %q, want %q", cheats[0].Description, want)
	}
}

func TestParseTagsApplyUntilRedefined(t *testing.T) {
	in := `% git, branch
# one
git branch

% git, log
# two
git log
`
	cheats, err := Parse(strings.NewReader(in), "git.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 2 {
		t.Fatalf("got %d cheats, want 2", len(cheats))
	}
	if want := []string{"git", "branch"}; !equal(cheats[0].Tags, want) {
		t.Errorf("cheats[0].Tags = %v, want %v", cheats[0].Tags, want)
	}
	if want := []string{"git", "log"}; !equal(cheats[1].Tags, want) {
		t.Errorf("cheats[1].Tags = %v, want %v", cheats[1].Tags, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantLines []int
		wantMsg   string
	}{
		{
			name:      "command before tags",
			in:        "# a description\nls -l\n",
			wantLines: []int{2},
			wantMsg:   "before any % tags line",
		},
		{
			name:      "command without description",
			in:        "% shell\nls -l\n",
			wantLines: []int{2},
			wantMsg:   "no # description line",
		},
		{
			name:      "second command reuses a consumed description",
			in:        "% shell\n# list files\nls\nls -l\n",
			wantLines: []int{4},
			wantMsg:   "no # description line",
		},
		{
			name:      "dangling description before blank line",
			in:        "% shell\n# nothing follows\n\n# list files\nls\n",
			wantLines: []int{3},
			wantMsg:   "not followed by a command",
		},
		{
			name:      "dangling description at end of file",
			in:        "% shell\n# list files\nls\n# nothing follows\n",
			wantLines: []int{4},
			wantMsg:   "end of file",
		},
		{
			name:      "empty tags line",
			in:        "%  , ,\n# list files\nls\n",
			wantLines: []int{1, 3},
			wantMsg:   "has no tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.in), "test.cheat")
			if err == nil {
				t.Fatal("Parse succeeded, want an error")
			}

			var errs ParseErrors
			if !errors.As(err, &errs) {
				t.Fatalf("error is %T, want ParseErrors", err)
			}
			if len(errs) != len(tt.wantLines) {
				t.Fatalf("got %d errors (%v), want %d", len(errs), err, len(tt.wantLines))
			}
			for i, line := range tt.wantLines {
				if errs[i].Line != line {
					t.Errorf("errs[%d].Line = %d, want %d", i, errs[i].Line, line)
				}
				if errs[i].Source != "test.cheat" {
					t.Errorf("errs[%d].Source = %q, want %q", i, errs[i].Source, "test.cheat")
				}
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestParseReturnsValidCheatsAlongsideErrors(t *testing.T) {
	in := `% shell
# list files
ls
ls -l
# print the working directory
pwd
`
	cheats, err := Parse(strings.NewReader(in), "test.cheat")
	if err == nil {
		t.Fatal("Parse succeeded, want an error for the bare 'ls -l' line")
	}
	if len(cheats) != 2 {
		t.Fatalf("got %d cheats, want 2 (ls and pwd)", len(cheats))
	}
	if cheats[0].Command != "ls" || cheats[1].Command != "pwd" {
		t.Errorf("got commands %q and %q, want \"ls\" and \"pwd\"", cheats[0].Command, cheats[1].Command)
	}
}

func TestParseEmptyInput(t *testing.T) {
	cheats, err := Parse(strings.NewReader(""), "empty.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 0 {
		t.Fatalf("got %d cheats, want 0", len(cheats))
	}
}

func TestCheatString(t *testing.T) {
	c := Cheat{Tags: []string{"git", "log"}, Description: "Show history"}
	if want := "git,log: Show history"; c.String() != want {
		t.Errorf("String() = %q, want %q", c.String(), want)
	}
}

func TestParseErrorMessageFormat(t *testing.T) {
	e := &ParseError{Source: "a/b.cheat", Line: 7, Msg: "boom"}
	if want := "a/b.cheat:7: boom"; e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
