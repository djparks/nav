package cheat

import (
	"bufio"
	"fmt"
	"io"
	"strings"
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
	)

	addErr := func(line int, format string, args ...any) {
		errs = append(errs, &ParseError{Source: source, Line: line, Msg: fmt.Sprintf(format, args...)})
	}

	scanner := bufio.NewScanner(r)
	// Cheat files are small, but a long one-liner should not break the parser.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())

		switch {
		case text == "":
			// Blank lines separate blocks; a dangling description is a mistake.
			if len(description) > 0 {
				addErr(line, "description is not followed by a command")
				description = nil
			}

		case strings.HasPrefix(text, ";"):
			// Comment for the reader of the file. Ignored.

		case strings.HasPrefix(text, "%"):
			if len(description) > 0 {
				addErr(line, "description is not followed by a command")
				description = nil
			}
			parsed := parseTags(text[1:])
			if len(parsed) == 0 {
				addErr(line, "tags line %q has no tags", text)
				continue
			}
			tags = parsed
			seenTags = true

		case strings.HasPrefix(text, "#"):
			d := strings.TrimSpace(text[1:])
			if d != "" {
				description = append(description, d)
			}

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
			cheats = append(cheats, Cheat{
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
