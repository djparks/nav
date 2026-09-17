package ui

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"nav/internal/term"
)

// prompt drives Prompt with a scripted sequence of keypress bytes.
func prompt(t *testing.T, names []string, keys string, opts ...func(*PromptOptions)) (map[string]string, bool, string) {
	t.Helper()

	var out bytes.Buffer
	o := PromptOptions{
		Input:  strings.NewReader(keys),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 24, Cols: 80} },
	}
	for _, f := range opts {
		f(&o)
	}

	values, ok, err := Prompt(names, o)
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}
	return values, ok, out.String()
}

func withValues(v map[string][]string) func(*PromptOptions) {
	return func(o *PromptOptions) { o.Values = v }
}

func TestPromptFreeText(t *testing.T) {
	values, ok, _ := prompt(t, []string{"count"}, "42\r")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["count"] != "42" {
		t.Errorf(`values["count"] = %q, want "42"`, values["count"])
	}
}

func TestPromptSelectsAPredefinedValue(t *testing.T) {
	// Enter with no typing accepts the first predefined value.
	values, ok, _ := prompt(t, []string{"format"}, "\r",
		withValues(map[string][]string{"format": {"table", "json", "yaml"}}))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["format"] != "table" {
		t.Errorf(`values["format"] = %q, want "table"`, values["format"])
	}
}

func TestPromptMovesThroughPredefinedValues(t *testing.T) {
	values, _, _ := prompt(t, []string{"format"}, "\x1b[B\x1b[B\r",
		withValues(map[string][]string{"format": {"table", "json", "yaml"}}))
	if values["format"] != "yaml" {
		t.Errorf(`values["format"] = %q, want "yaml"`, values["format"])
	}
}

func TestPromptFiltersPredefinedValues(t *testing.T) {
	// Typing narrows the list; Enter takes the only remaining entry.
	values, _, _ := prompt(t, []string{"format"}, "ya\r",
		withValues(map[string][]string{"format": {"table", "json", "yaml"}}))
	if values["format"] != "yaml" {
		t.Errorf(`values["format"] = %q, want "yaml"`, values["format"])
	}
}

func TestPromptFilteringIsCaseInsensitive(t *testing.T) {
	values, _, _ := prompt(t, []string{"format"}, "JS\r",
		withValues(map[string][]string{"format": {"table", "json", "yaml"}}))
	if values["format"] != "json" {
		t.Errorf(`values["format"] = %q, want "json"`, values["format"])
	}
}

func TestPromptAcceptsTextOutsideThePredefinedList(t *testing.T) {
	// A value that matches nothing empties the list, and Enter then takes
	// the typed text. Predefined values are suggestions, not a restriction.
	values, ok, out := prompt(t, []string{"format"}, "custom-thing\r",
		withValues(map[string][]string{"format": {"table", "json"}}))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["format"] != "custom-thing" {
		t.Errorf(`values["format"] = %q, want "custom-thing"`, values["format"])
	}
	if !strings.Contains(out, "uses what you typed") {
		t.Errorf("output does not tell the user the typed text will be used:\n%s", out)
	}
}

func TestPromptHandlesSeveralVariablesInOrder(t *testing.T) {
	// Each Enter moves to the next variable, so one script fills them all.
	values, ok, _ := prompt(t, []string{"count", "branch"}, "5\rmain\r")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["count"] != "5" || values["branch"] != "main" {
		t.Errorf("values = %v, want count=5 and branch=main", values)
	}
}

func TestPromptMixesListAndFreeTextVariables(t *testing.T) {
	values, ok, _ := prompt(t, []string{"format", "name"}, "json\rweb\r",
		withValues(map[string][]string{"format": {"table", "json"}}))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["format"] != "json" {
		t.Errorf(`values["format"] = %q, want "json"`, values["format"])
	}
	if values["name"] != "web" {
		t.Errorf(`values["name"] = %q, want "web"`, values["name"])
	}
}

func TestPromptCancelling(t *testing.T) {
	for name, keys := range map[string]string{
		"esc":            "\x1b",
		"ctrl-c":         "\x03",
		"end of input":   "",
		"esc on the 2nd": "5\r\x1b",
	} {
		t.Run(name, func(t *testing.T) {
			values, ok, _ := prompt(t, []string{"count", "branch"}, keys)
			if ok {
				t.Error("ok = true, want false after cancelling")
			}
			// Cancelling must not hand back a half-filled set of values, or
			// the caller could run an incomplete command.
			if values != nil {
				t.Errorf("values = %v, want nil after cancelling", values)
			}
		})
	}
}

func TestPromptBackspaceAndClear(t *testing.T) {
	values, _, _ := prompt(t, []string{"count"}, "49\x7f2\r")
	if values["count"] != "42" {
		t.Errorf(`values["count"] = %q, want "42"`, values["count"])
	}

	// Ctrl-U clears the box.
	values, _, _ = prompt(t, []string{"count"}, "999\x157\r")
	if values["count"] != "7" {
		t.Errorf(`values["count"] = %q, want "7"`, values["count"])
	}

	// Ctrl-W drops the last word.
	values, _, _ = prompt(t, []string{"msg"}, "hello world\x17there\r")
	if values["msg"] != "hello there" {
		t.Errorf(`values["msg"] = %q, want "hello there"`, values["msg"])
	}
}

func TestPromptAcceptsAnEmptyValue(t *testing.T) {
	// Pressing Enter straight away gives an empty value, which is a
	// legitimate thing to want for an optional flag.
	values, ok, _ := prompt(t, []string{"flags"}, "\r")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["flags"] != "" {
		t.Errorf(`values["flags"] = %q, want ""`, values["flags"])
	}
}

func TestPromptShowsTheCommandAndProgress(t *testing.T) {
	_, _, out := prompt(t, []string{"count", "branch"}, "\x1b",
		func(o *PromptOptions) { o.Command = "git log -n <count> <branch>" })

	for _, want := range []string{
		"Command:",
		"git log -n <count> <branch>",
		"count:",
		"variable 1 of 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestPromptShowsValuesFilledSoFar(t *testing.T) {
	// While asking for the second variable, the preview should already show
	// the first one's value rather than its placeholder.
	_, _, out := prompt(t, []string{"count", "branch"}, "5\r\x1b",
		func(o *PromptOptions) { o.Command = "git log -n <count> <branch>" })

	if !strings.Contains(out, "git log -n 5 <branch>") {
		t.Errorf("output does not preview the value chosen so far:\n%s", out)
	}
	if !strings.Contains(out, "variable 2 of 2") {
		t.Errorf("output does not show progress through the variables:\n%s", out)
	}
}

func TestPromptListScrolls(t *testing.T) {
	// Forty values in a terminal with room for far fewer: End must scroll
	// to the last one rather than run off the bottom.
	many := make([]string, 40)
	for i := range many {
		many[i] = "value-" + strconv.Itoa(i)
	}

	values, ok, out := prompt(t, []string{"v"}, "\x1b[F\r",
		withValues(map[string][]string{"v": many}))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if values["v"] != many[39] {
		t.Errorf(`values["v"] = %q, want %q`, values["v"], many[39])
	}

	frames := strings.Split(out, ansiHome)
	if !strings.Contains(frames[len(frames)-1], many[39]) {
		t.Error("the highlighted value is not visible in the final frame")
	}
}

func TestPromptRendersWithinTheTerminal(t *testing.T) {
	var out bytes.Buffer
	_, _, err := Prompt([]string{"v"}, PromptOptions{
		Input:  strings.NewReader("\x1b"),
		Output: &out,
		Values: map[string][]string{"v": {"one", "two", "three", "four", "five", "six"}},
		Size:   func() term.Size { return term.Size{Rows: 12, Cols: 40} },
	})
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}

	frame := strings.Split(out.String(), ansiHome)[1]
	if rows := strings.Count(frame, "\r\n"); rows > 12 {
		t.Errorf("drew %d rows in a 12-row terminal", rows)
	}
	// Raw mode needs every newline paired with a carriage return.
	for i, r := range out.String() {
		if r == '\n' && (i == 0 || out.String()[i-1] != '\r') {
			t.Fatalf("found a \\n not preceded by \\r at byte %d", i)
		}
	}
}

func TestPromptTruncatesLongValues(t *testing.T) {
	long := strings.Repeat("v", 200)
	var out bytes.Buffer
	_, _, err := Prompt([]string{"v"}, PromptOptions{
		Input:  strings.NewReader("\x1b"),
		Output: &out,
		Values: map[string][]string{"v": {long}},
		Size:   func() term.Size { return term.Size{Rows: 24, Cols: 40} },
	})
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}
	if strings.Contains(out.String(), strings.Repeat("v", 45)) {
		t.Error("a long value was not truncated to the terminal width")
	}
}

func TestPromptWithNoNames(t *testing.T) {
	// Nothing to ask for: Prompt should return immediately without reading
	// any input or drawing anything.
	var out bytes.Buffer
	values, ok, err := Prompt(nil, PromptOptions{
		Input:  strings.NewReader(""),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
	if len(values) != 0 {
		t.Errorf("values = %v, want empty", values)
	}
	if out.Len() != 0 {
		t.Errorf("Prompt drew something with no variables to ask about: %q", out.String())
	}
}

func TestPromptRequiresInputAndOutput(t *testing.T) {
	if _, _, err := Prompt([]string{"v"}, PromptOptions{Output: &bytes.Buffer{}}); err == nil {
		t.Error("Prompt with no Input succeeded, want an error")
	}
	if _, _, err := Prompt([]string{"v"}, PromptOptions{Input: strings.NewReader("")}); err == nil {
		t.Error("Prompt with no Output succeeded, want an error")
	}
}
