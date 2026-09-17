package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"nav/internal/term"
	"nav/internal/variables"
)

// promptReserved is the chrome around a value list: the command being built,
// a blank line, the variable name and its input, a blank line, a blank line
// and the footer.
const promptReserved = 7

// PromptOptions configures a run of Prompt.
type PromptOptions struct {
	// Input supplies raw keypress bytes. Required.
	Input io.Reader
	// Output receives rendered frames. Required.
	Output io.Writer
	// Command is the command being filled in, shown above the prompt so the
	// user can see what the value is for.
	Command string
	// Values holds the predefined values for each variable name. A variable
	// with no entry, or an empty one, gets a free-text prompt.
	Values map[string][]string
	// Size reports the terminal size before each frame. Defaults to
	// term.FallbackSize.
	Size func() term.Size
}

// Prompt asks for a value for each of names, in order, and returns them
// keyed by name.
//
// ok is false if the user backed out, in which case the caller should abandon
// the command rather than run it half-filled.
func Prompt(names []string, opts PromptOptions) (values map[string]string, ok bool, err error) {
	if opts.Input == nil || opts.Output == nil {
		return nil, false, errors.New("ui.Prompt: Input and Output are required")
	}
	return runPrompts(newKeyReader(opts.Input), names, opts)
}

// runPrompts is Prompt with the input reader supplied by the caller. Every
// variable is asked through the same reader, so a value typed ahead of its
// prompt is not lost between them.
func runPrompts(kr *keyReader, names []string, opts PromptOptions) (map[string]string, bool, error) {
	size := opts.Size
	if size == nil {
		size = func() term.Size { return term.FallbackSize }
	}

	values := make(map[string]string, len(names))
	for i, name := range names {
		p := &valuePrompt{
			name:     name,
			options:  opts.Values[name],
			command:  opts.Command,
			position: i + 1,
			total:    len(names),
			// Show the command with the values chosen so far already filled
			// in, so progress is visible as the user works through it.
			filled: values,
			size:   size,
			out:    opts.Output,
		}
		p.refilter()

		act, err := runLoop(kr, p)
		if err != nil {
			return nil, false, err
		}
		if act != actionAccept {
			return nil, false, nil
		}
		values[name] = p.value()
	}
	return values, true, nil
}

// valuePrompt asks for one variable's value.
//
// With predefined values it behaves like the cheat selector: a filterable
// list. Without them it is a plain text box. The two are one view because
// typing filters the list in the first case and builds the value in the
// second, and the key handling is otherwise identical.
type valuePrompt struct {
	name     string
	options  []string // predefined values; empty means free text
	command  string
	position int               // which variable this is, 1-based
	total    int               // how many variables in all
	filled   map[string]string // values chosen so far

	input   []rune
	matches []string
	cursor  int
	offset  int

	size func() term.Size
	out  io.Writer
}

// hasList reports whether this prompt shows a list of predefined values.
func (p *valuePrompt) hasList() bool { return len(p.options) > 0 }

// value is the value the prompt has settled on.
//
// A highlighted list entry wins. Otherwise the typed text is used, which is
// what makes a value outside the predefined list possible: type something no
// entry matches and the list empties out, leaving the text to be accepted.
func (p *valuePrompt) value() string {
	if p.hasList() && len(p.matches) > 0 {
		return p.matches[p.cursor]
	}
	return string(p.input)
}

func (p *valuePrompt) handleKey(k key) action {
	switch k.kind {
	case keyQuit:
		return actionCancel

	case keyEnter:
		return actionAccept

	case keyRune:
		p.input = append(p.input, k.r)
		p.refilter()

	case keyBackspace:
		if len(p.input) > 0 {
			p.input = p.input[:len(p.input)-1]
			p.refilter()
		}

	case keyClearQuery:
		if len(p.input) > 0 {
			p.input = p.input[:0]
			p.refilter()
		}

	case keyDeleteWord:
		p.deleteWord()

	case keyUp:
		p.move(-1)
	case keyDown:
		p.move(1)
	case keyPageUp:
		p.move(-p.visibleRows())
	case keyPageDown:
		p.move(p.visibleRows())
	case keyHome:
		p.move(-len(p.matches))
	case keyEnd:
		p.move(len(p.matches))
	}
	return actionContinue
}

// deleteWord removes the last whitespace-delimited word of the input.
func (p *valuePrompt) deleteWord() {
	i := len(p.input)
	for i > 0 && p.input[i-1] == ' ' {
		i--
	}
	for i > 0 && p.input[i-1] != ' ' {
		i--
	}
	if i != len(p.input) {
		p.input = p.input[:i]
		p.refilter()
	}
}

// refilter narrows the predefined values to those containing the typed text,
// case-insensitively.
func (p *valuePrompt) refilter() {
	p.cursor, p.offset = 0, 0
	if !p.hasList() {
		p.matches = nil
		return
	}

	needle := strings.ToLower(strings.TrimSpace(string(p.input)))
	p.matches = make([]string, 0, len(p.options))
	for _, opt := range p.options {
		if needle == "" || strings.Contains(strings.ToLower(opt), needle) {
			p.matches = append(p.matches, opt)
		}
	}
}

func (p *valuePrompt) move(delta int) {
	p.cursor, p.offset = moveInList(p.cursor, p.offset, delta, len(p.matches), p.visibleRows())
}

// visibleRows is how many list entries fit on screen at the current size.
func (p *valuePrompt) visibleRows() int {
	return max(p.size().Rows-promptReserved, 1)
}

func (p *valuePrompt) render() error {
	size := p.size()
	width := max(size.Cols, 20)

	sc := newScreen(width)

	// The command so far, with the values already chosen filled in.
	sc.line(ansiDim + "Command:" + ansiReset)
	sc.line("  " + truncate(variables.Substitute(p.command, p.filled), width-2))
	sc.line("")

	label := p.name + ":"
	sc.line(ansiBold + truncate(label, width) + ansiReset)
	sc.line("  " + truncate(string(p.input), width-2) + ansiReverse + " " + ansiReset)
	sc.line("")

	rows := p.visibleRows()
	switch {
	case !p.hasList():
		// Free text: nothing to list, so leave the space empty.
		sc.blank(rows)
	case len(p.matches) == 0:
		sc.line(ansiDim + "  no predefined value matches; ⏎ uses what you typed" + ansiReset)
		sc.blank(rows - 1)
	default:
		end := min(p.offset+rows, len(p.matches))
		for i := p.offset; i < end; i++ {
			if i == p.cursor {
				sc.line(ansiReverse + "> " + truncate(p.matches[i], width-2) + ansiReset)
			} else {
				sc.line("  " + truncate(p.matches[i], width-2))
			}
		}
		sc.blank(rows - (end - p.offset))
	}

	sc.line(ansiDim + truncate(p.footer(), width) + ansiReset)
	return sc.flush(p.out)
}

func (p *valuePrompt) footer() string {
	progress := fmt.Sprintf("variable %d of %d", p.position, p.total)

	var hints string
	switch {
	case !p.hasList():
		hints = "type a value   ⏎ accept   esc cancel"
	case len(p.matches) == 0:
		hints = "⏎ use typed text   esc cancel"
	default:
		hints = fmt.Sprintf("%d of %d   ↑↓ move   ⏎ accept   esc cancel",
			p.cursor+1, len(p.matches))
	}
	return progress + "   " + hints
}
