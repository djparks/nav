// Package search filters cheats by a text query.
//
// The query is split on whitespace into terms, and a cheat matches only if
// *every* term is found somewhere in it — so "docker ps" matches a cheat that
// mentions both, in any order and in any field. Matching is case-insensitive
// substring matching, which is predictable and needs no scoring.
//
// A term may be restricted to one field with a prefix:
//
//	tag:git      match only against the cheat's tags
//	desc:branch  match only against the description
//	cmd:rev-parse match only against the command text
//
// An unprefixed term matches against any of the three.
package search

import (
	"strings"

	"nav/internal/cheat"
)

// Field identifies the parts of a cheat that a term may match against. The
// values are bit flags so they can be combined.
type Field uint8

const (
	FieldTags Field = 1 << iota
	FieldDescription
	FieldCommand

	// AnyField is the default: match against tags, description and command.
	AnyField = FieldTags | FieldDescription | FieldCommand
)

// fieldPrefixes maps the query prefixes to the field they restrict a term to.
var fieldPrefixes = []struct {
	prefix string
	field  Field
}{
	{"tag:", FieldTags},
	{"tags:", FieldTags},
	{"desc:", FieldDescription},
	{"description:", FieldDescription},
	{"cmd:", FieldCommand},
	{"command:", FieldCommand},
}

// term is one word of a query together with the fields it may match.
type term struct {
	text   string // already lower-cased
	fields Field
}

// Query is a parsed, ready-to-apply search query.
type Query struct {
	terms []term
}

// Parse splits a raw query string into terms. An empty or whitespace-only
// query matches everything.
func Parse(raw string) Query {
	var q Query
	for _, word := range strings.Fields(strings.ToLower(raw)) {
		t := term{text: word, fields: AnyField}
		for _, fp := range fieldPrefixes {
			if rest := strings.TrimPrefix(word, fp.prefix); rest != word {
				// A bare prefix such as "tag:" restricts nothing usefully.
				if rest == "" {
					t.text = ""
					break
				}
				t.text, t.fields = rest, fp.field
				break
			}
		}
		if t.text != "" {
			q.terms = append(q.terms, t)
		}
	}
	return q
}

// IsEmpty reports whether the query has no terms, in which case it matches
// every cheat.
func (q Query) IsEmpty() bool { return len(q.terms) == 0 }

// Matches reports whether c satisfies every term in the query.
func (q Query) Matches(c cheat.Cheat) bool {
	tags := strings.ToLower(strings.Join(c.Tags, " "))
	desc := strings.ToLower(c.Description)
	cmd := strings.ToLower(c.Command)

	for _, t := range q.terms {
		if !t.matches(tags, desc, cmd) {
			return false
		}
	}
	return true
}

// matches reports whether the term is present in any of the fields it is
// allowed to look at.
func (t term) matches(tags, desc, cmd string) bool {
	if t.fields&FieldTags != 0 && strings.Contains(tags, t.text) {
		return true
	}
	if t.fields&FieldDescription != 0 && strings.Contains(desc, t.text) {
		return true
	}
	if t.fields&FieldCommand != 0 && strings.Contains(cmd, t.text) {
		return true
	}
	return false
}

// Filter returns the cheats that match the query, in their original order.
// The result is always non-nil so callers can range over it freely.
func (q Query) Filter(cheats []cheat.Cheat) []cheat.Cheat {
	matches := make([]cheat.Cheat, 0, len(cheats))
	for _, c := range cheats {
		if q.Matches(c) {
			matches = append(matches, c)
		}
	}
	return matches
}

// Filter parses raw and applies it to cheats in one step.
func Filter(cheats []cheat.Cheat, raw string) []cheat.Cheat {
	return Parse(raw).Filter(cheats)
}
