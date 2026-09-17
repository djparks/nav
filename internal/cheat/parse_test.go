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

func TestParsePredefinedValues(t *testing.T) {
	in := `% docker, containers

# List running containers
docker ps --format "<format>"

$ format:
    table {{.Names}}	{{.Status}}
    json
    yaml
`
	cheats, err := Parse(strings.NewReader(in), "docker.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 1 {
		t.Fatalf("got %d cheats, want 1", len(cheats))
	}

	got := cheats[0].Values["format"]
	want := []string{"table {{.Names}}\t{{.Status}}", "json", "yaml"}
	if len(got) != len(want) {
		t.Fatalf("got %d values (%q), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("value %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseValuesKeepInnerPunctuation(t *testing.T) {
	// One value per line means there is no delimiter to escape, so commas,
	// pipes and colons survive untouched. That is the whole point of the
	// indented-block syntax.
	in := "% x\n# c\ncmd <v>\n\n$ v:\n    a,b|c: d\n"

	cheats, err := Parse(strings.NewReader(in), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if got := cheats[0].Values["v"]; len(got) != 1 || got[0] != "a,b|c: d" {
		t.Errorf("Values[\"v\"] = %q, want [\"a,b|c: d\"]", got)
	}
}

func TestParseValuesApplyToTheWholeSection(t *testing.T) {
	// A `$` block applies to every cheat in its `%` section, including ones
	// written above it.
	in := `% git, branch

# Show a branch
git show <branch>

# Delete a branch
git branch -d <branch>

$ branch:
    main
    develop
`
	cheats, err := Parse(strings.NewReader(in), "git.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 2 {
		t.Fatalf("got %d cheats, want 2", len(cheats))
	}
	for i, c := range cheats {
		if len(c.Values["branch"]) != 2 {
			t.Errorf("cheats[%d] has %d values for branch, want 2", i, len(c.Values["branch"]))
		}
	}
}

func TestParseValuesDoNotLeakBetweenSections(t *testing.T) {
	in := `% first
# a
cmd-a <v>

$ v:
    from-first

% second
# b
cmd-b <v>
`
	cheats, err := Parse(strings.NewReader(in), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 2 {
		t.Fatalf("got %d cheats, want 2", len(cheats))
	}
	if len(cheats[0].Values["v"]) != 1 {
		t.Errorf("the first section lost its values: %q", cheats[0].Values)
	}
	if cheats[1].Values != nil {
		t.Errorf("values leaked into the second section: %q", cheats[1].Values)
	}
}

func TestParseValueBlockEndings(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"a blank line ends the block", "% x\n# c\ncmd <v>\n\n$ v:\n    one\n\n# d\ncmd2\n"},
		{"an unindented line ends the block", "% x\n# c\ncmd <v>\n\n$ v:\n    one\n# d\ncmd2\n"},
		{"a new $ block ends the previous one", "% x\n# c\ncmd <v>\n\n$ v:\n    one\n$ w:\n    two\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cheats, err := Parse(strings.NewReader(tt.in), "t.cheat")
			if err != nil {
				t.Fatalf("Parse returned an error: %v", err)
			}
			if got := cheats[0].Values["v"]; len(got) != 1 || got[0] != "one" {
				t.Errorf("Values[\"v\"] = %q, want [\"one\"]", got)
			}
		})
	}
}

func TestParseCommentInsideValueBlock(t *testing.T) {
	// A `;` comment is ignored wherever it appears, including in the middle
	// of a value block, which it must not interrupt.
	in := "% x\n# c\ncmd <v>\n\n$ v:\n    one\n; a note\n    two\n"

	cheats, err := Parse(strings.NewReader(in), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if got := cheats[0].Values["v"]; len(got) != 2 {
		t.Errorf("Values[\"v\"] = %q, want two values", got)
	}
}

func TestParseRepeatedVariableBlockAccumulates(t *testing.T) {
	in := "% x\n# c\ncmd <v>\n\n$ v:\n    one\n\n$ v:\n    two\n"

	cheats, err := Parse(strings.NewReader(in), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if got := cheats[0].Values["v"]; len(got) != 2 {
		t.Errorf("Values[\"v\"] = %q, want both values", got)
	}
}

func TestParseIndentedCommandIsStillACommand(t *testing.T) {
	// Indentation is only meaningful directly below a `$` header, so an
	// indented command keeps working as it did before variables existed.
	in := "% x\n# c\n    ls -l\n"

	cheats, err := Parse(strings.NewReader(in), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 1 || cheats[0].Command != "ls -l" {
		t.Fatalf("got %d cheats (%v), want the indented command", len(cheats), cheats)
	}
}

func TestParseValueErrors(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantLines []int
		wantMsg   string
	}{
		{
			name:      "no values listed",
			in:        "% x\n# c\ncmd <v>\n\n$ v:\n",
			wantLines: []int{5},
			wantMsg:   `variable "v" has no values`,
		},
		{
			name:      "no values before the next block",
			in:        "% x\n# c\ncmd <v>\n\n$ v:\n$ w:\n    two\n",
			wantLines: []int{5},
			wantMsg:   "has no values",
		},
		{
			name:      "missing colon",
			in:        "% x\n# c\ncmd <v>\n\n$ v\n    one\n",
			wantLines: []int{5},
			wantMsg:   "must end with a colon",
		},
		{
			name:      "no name",
			in:        "% x\n# c\ncmd <v>\n\n$ :\n    one\n",
			wantLines: []int{5},
			wantMsg:   "has no name",
		},
		{
			name:      "unusable name",
			in:        "% x\n# c\ncmd <v>\n\n$ 1bad:\n    one\n",
			wantLines: []int{5},
			wantMsg:   "is not usable",
		},
		{
			name:      "a $ block does not satisfy a pending description",
			in:        "% x\n# dangling\n$ v:\n    one\n",
			wantLines: []int{3},
			wantMsg:   "not followed by a command",
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
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestParseNoValuesLeavesValuesNil(t *testing.T) {
	cheats, err := Parse(strings.NewReader("% x\n# c\nls\n"), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if cheats[0].Values != nil {
		t.Errorf("Values = %v, want nil when no $ block is present", cheats[0].Values)
	}
}

func TestParseVariableWithoutValuesIsFine(t *testing.T) {
	// A command may use a variable that has no `$` block; the user is asked
	// to type it. That is not a parse error.
	cheats, err := Parse(strings.NewReader("% x\n# c\ngit show <commit>\n"), "t.cheat")
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if len(cheats) != 1 {
		t.Fatalf("got %d cheats, want 1", len(cheats))
	}
}
