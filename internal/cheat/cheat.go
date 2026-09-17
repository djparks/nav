// Package cheat defines the cheatsheet data model and the parser for
// `.cheat` files.
//
// A cheatsheet file is plain text. Five kinds of lines exist:
//
//	% git, version-control      tags for every cheat that follows
//	# Show the current branch   description of the next command
//	git rev-parse --abbrev-ref HEAD
//	; anything after a semicolon is ignored
//	$ branch:                   predefined values for <branch>, one per
//	    main                    indented line below
//	    develop
//
// Blank lines and `;` comments are ignored. Every command line must be
// preceded by at least one `#` description line, and a `%` tags line must
// appear before the first command in the file.
package cheat

import "strings"

// Cheat is a single runnable snippet together with the metadata that makes
// it findable.
type Cheat struct {
	// Tags come from the nearest preceding `%` line, e.g. ["git", "log"].
	Tags []string
	// Description is the text of the `#` line(s) above the command. Multiple
	// comment lines are joined with a single space.
	Description string
	// Command is the snippet itself, exactly as written in the file.
	Command string
	// Source is the path of the file the cheat was read from.
	Source string
	// Line is the 1-based line number of the command within Source.
	Line int
	// Values holds the predefined values for `<variable>` placeholders,
	// keyed by variable name, as declared by the `$ name:` blocks in the
	// same `%` tag section. It is nil when the section declared none.
	//
	// The map is shared by every cheat in a section and must be treated as
	// read-only.
	Values map[string][]string
}

// String renders the cheat the way it is shown in a selection list:
// "tag1,tag2: description".
func (c Cheat) String() string {
	return strings.Join(c.Tags, ",") + ": " + c.Description
}
