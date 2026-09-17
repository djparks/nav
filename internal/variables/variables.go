// Package variables finds and fills in the `<placeholder>` parts of a
// command.
//
// A variable is a name wrapped in angle brackets, as in:
//
//	docker ps --format "<format>"
//	git log -n <count> <branch>
//
// A name starts with a letter or underscore and continues with letters,
// digits, underscores or hyphens. Requiring that shape keeps ordinary shell
// syntax from being mistaken for a variable: shell redirection (`< file`),
// process substitution and comparisons all contain a `<` that is not
// followed by a bare name and a `>`.
//
// This package owns the syntax. The cheatsheet parser validates the names in
// `$ name:` blocks against IsValidName so the two can never disagree about
// what counts as a variable.
package variables

import (
	"regexp"
	"strings"
)

// pattern matches a single `<name>` placeholder and captures the name.
var pattern = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_-]*)>`)

// nameOnly is pattern anchored to a whole string, for IsValidName.
var nameOnly = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// IsValidName reports whether s is a usable variable name.
func IsValidName(s string) bool {
	return nameOnly.MatchString(s)
}

// Names returns the variables used by command, in the order they first
// appear, with repeats removed. A command that uses `<file>` twice yields
// "file" once, so the user is asked for it once.
//
// The result is nil when the command uses no variables.
func Names(command string) []string {
	matches := pattern.FindAllStringSubmatch(command, -1)
	if matches == nil {
		return nil
	}

	var (
		names []string
		seen  = make(map[string]bool, len(matches))
	)
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// Substitute replaces every `<name>` in command with values[name].
//
// Placeholders with no entry in values are left exactly as they were, so a
// partially filled command is still readable rather than becoming a command
// with holes punched in it.
func Substitute(command string, values map[string]string) string {
	if len(values) == 0 {
		return command
	}
	return pattern.ReplaceAllStringFunc(command, func(match string) string {
		name := strings.Trim(match, "<>")
		if v, ok := values[name]; ok {
			return v
		}
		return match
	})
}
