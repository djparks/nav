package cheat

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"nav/internal/variables"
)

// ParseError describes a single problem found while reading a cheatsheet.
// Parsing does not stop at the first problem, so a caller can show the user
// everything that is wrong with a file in one go.
type ParseError struct {
	Source string // file the problem was found in
	Line   int    // 1-based line number
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.Source, e.Line, e.Msg)
}

// ParseErrors is the collection of problems found in one or more files.
type ParseErrors []*ParseError

func (errs ParseErrors) Error() string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Parse reads a cheatsheet from r. source is used only for error messages
// and to populate Cheat.Source.
//
// Cheats found before an error are still returned, so the caller can decide
// whether a partially valid file is good enough. The returned error, when
// non-nil, is always of type ParseErrors.
func Parse(r io.Reader, source string) ([]Cheat, error) {
	var (
		cheats []Cheat
		errs   ParseErrors

		tags        []string
		seenTags    bool
		description []string

		// `$ name:` blocks are scoped to the current `%` section and apply
		// to every cheat in it, including ones written above the block. So
		// a section's cheats are held back until the section ends and its
		// values are known.
		section       []Cheat
		sectionValues map[string][]string

		// openVar is the name of the `$` block currently accepting indented
		// value lines, or "" when none is open. openVarLine is where it
		// started, for the "no values" error.
		openVar     string
		openVarLine int

		// skippingVar is set when a `$` header was rejected, so its value
		// lines are discarded rather than mistaken for commands. One bad
		// block should produce one error, not a cascade.
		skippingVar bool
	)

	addErr := func(line int, format string, args ...any) {
		errs = append(errs, &ParseError{Source: source, Line: line, Msg: fmt.Sprintf(format, args...)})
	}

	// closeVar ends an open `$` block, complaining if it listed no values.
	closeVar := func() {
		if openVar != "" && len(sectionValues[openVar]) == 0 {
			addErr(openVarLine, "variable %q has no values; indent them on the lines below", openVar)
			delete(sectionValues, openVar)
		}
		openVar = ""
	}

	// endSection flushes the buffered cheats, attaching the section's
	// predefined values to each of them.
	endSection := func() {
		closeVar()
		for i := range section {
			if len(sectionValues) > 0 {
				section[i].Values = sectionValues
			}
			cheats = append(cheats, section[i])
		}
		section = nil
		sectionValues = nil
	}

	scanner := bufio.NewScanner(r)
	// Cheat files are small, but a long one-liner should not break the parser.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	line := 0
	for scanner.Scan() {
		line++
		// The raw line is kept because leading whitespace is what marks a
		// predefined value inside a `$` block.
		raw := scanner.Text()
		text := strings.TrimSpace(raw)
		indented := text != "" && (raw[0] == ' ' || raw[0] == '\t')

		// An indented line directly below a `$ name:` header is one of its
		// values. Checked first, so a value may look like anything at all.
		if indented {
			if openVar != "" {
				sectionValues[openVar] = append(sectionValues[openVar], text)
				continue
			}
			if skippingVar {
				continue
			}
			// Otherwise indentation carries no meaning, and the line is
			// handled below like any other.
		}
		// Every remaining line is unindented, which ends a rejected block.
		skippingVar = false

		switch {
		case text == "":
			// Blank lines separate blocks; a dangling description is a mistake.
			closeVar()
			if len(description) > 0 {
				addErr(line, "description is not followed by a command")
				description = nil
			}

		case strings.HasPrefix(text, ";"):
			// Comment for the reader of the file. Ignored, and it does not
			// interrupt a `$` block.

		case strings.HasPrefix(text, "%"):
			if len(description) > 0 {
				addErr(line, "description is not followed by a command")
				description = nil
			}
			endSection()
			parsed := parseTags(text[1:])
			if len(parsed) == 0 {
				addErr(line, "tags line %q has no tags", text)
				continue
			}
			tags = parsed
			seenTags = true

		case strings.HasPrefix(text, "#"):
			closeVar()
			d := strings.TrimSpace(text[1:])
			if d != "" {
				description = append(description, d)
			}

		case strings.HasPrefix(text, "$"):
			closeVar()
			if len(description) > 0 {
				addErr(line, "description is not followed by a command")
				description = nil
			}
			name, err := parseVarName(text)
			if err != nil {
				addErr(line, "%s", err)
				skippingVar = true
				continue
			}
			if sectionValues == nil {
				sectionValues = make(map[string][]string)
			}
			// An existing entry is kept, so a name may be declared twice
			// and the values accumulate.
			if _, ok := sectionValues[name]; !ok {
				sectionValues[name] = nil
			}
			openVar, openVarLine = name, line

		default: // a command
			// On error, drop the pending description too: one bad block
			// should produce one error, not a cascade.
			if !seenTags {
				addErr(line, "command found before any %% tags line")
				description = nil
				continue
			}
			if len(description) == 0 {
				addErr(line, "command has no # description line above it")
				continue
			}
			section = append(section, Cheat{
				Tags:        tags,
				Description: strings.Join(description, " "),
				Command:     text,
				Source:      source,
				Line:        line,
			})
			// A description belongs to exactly one command.
			description = nil
		}
	}
	endSection()
	if err := scanner.Err(); err != nil {
		return cheats, ParseErrors{{Source: source, Msg: err.Error()}}
	}
	if len(description) > 0 {
		addErr(line, "description at end of file is not followed by a command")
	}

	if len(errs) > 0 {
		return cheats, errs
	}
	return cheats, nil
}

// parseVarName pulls the variable name out of a `$ name:` header line.
func parseVarName(text string) (string, error) {
	body := strings.TrimSpace(strings.TrimPrefix(text, "$"))

	name, found := strings.CutSuffix(body, ":")
	if !found {
		return "", fmt.Errorf("variable definition %q must end with a colon, as in \"$ name:\"", text)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("variable definition %q has no name", text)
	}
	if !variables.IsValidName(name) {
		return "", fmt.Errorf("variable name %q is not usable; names start with a letter or underscore "+
			"and continue with letters, digits, underscores or hyphens", name)
	}
	return name, nil
}

// parseTags splits a comma-separated tag list, dropping empty entries.
func parseTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
