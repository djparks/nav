package ui

import "unicode/utf8"

// keyKind names the keypresses the selector reacts to. Anything else is
// reported as keyUnknown and ignored.
type keyKind int

const (
	keyRune keyKind = iota // a printable character, typed into the query
	keyEnter
	keyBackspace
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
	keyQuit       // Ctrl-C or Esc
	keyCopy       // Ctrl-Y
	keyClearQuery // Ctrl-U
	keyDeleteWord // Ctrl-W
	keyUnknown
)

// key is a single decoded keypress.
type key struct {
	kind keyKind
	r    rune // only meaningful when kind is keyRune
}

// Control bytes, named so the decoder reads as prose.
const (
	ctrlC     = 0x03
	ctrlN     = 0x0e
	ctrlP     = 0x10
	ctrlU     = 0x15
	ctrlW     = 0x17
	ctrlY     = 0x19
	esc       = 0x1b
	backspace = 0x08
	del       = 0x7f
	lineFeed  = 0x0a
	carriage  = 0x0d
)

// decode reads the first keypress out of buf.
//
// It returns the key and how many bytes it consumed. ok is false when buf
// holds the start of a sequence but not all of it — an escape sequence split
// across two reads — and the caller should read more bytes and try again.
func decode(buf []byte) (k key, n int, ok bool) {
	if len(buf) == 0 {
		return key{}, 0, false
	}

	switch b := buf[0]; b {
	case ctrlC:
		return key{kind: keyQuit}, 1, true
	case carriage, lineFeed:
		return key{kind: keyEnter}, 1, true
	case del, backspace:
		return key{kind: keyBackspace}, 1, true
	case ctrlY:
		return key{kind: keyCopy}, 1, true
	case ctrlU:
		return key{kind: keyClearQuery}, 1, true
	case ctrlW:
		return key{kind: keyDeleteWord}, 1, true
	case ctrlP:
		return key{kind: keyUp}, 1, true
	case ctrlN:
		return key{kind: keyDown}, 1, true
	case esc:
		return decodeEscape(buf)
	}

	// Remaining C0 control bytes are not bound to anything.
	if buf[0] < 0x20 {
		return key{kind: keyUnknown}, 1, true
	}

	r, size := utf8.DecodeRune(buf)
	if r == utf8.RuneError && size <= 1 {
		// Either invalid UTF-8 or a multi-byte rune cut short by the read
		// boundary. Waiting for more bytes resolves the second case; if the
		// bytes really are invalid, treat them as one unknown key.
		if len(buf) < utf8.UTFMax {
			return key{}, 0, false
		}
		return key{kind: keyUnknown}, 1, true
	}
	return key{kind: keyRune, r: r}, size, true
}

// decodeFinal decodes the head of buf when it is known that no further
// bytes will arrive — because the input closed, or because the caller waited
// and nothing followed. Unlike decode it always produces a key.
//
// This is where a bare Esc becomes a quit: an Esc with nothing after it is
// the Esc key, whereas an Esc followed by more bytes is an arrow or page key.
func decodeFinal(buf []byte) (key, int) {
	if k, n, ok := decode(buf); ok {
		return k, n
	}
	if len(buf) == 0 {
		return key{kind: keyUnknown}, 0
	}
	// A leading Esc wins even if a sequence had started but never finished.
	// Quitting on Esc has to be dependable, and the alternative would be to
	// swallow the keypress silently.
	if buf[0] == esc {
		return key{kind: keyQuit}, 1
	}
	// A truncated multi-byte rune. There is nothing sensible to do with it,
	// so drop one byte and move on.
	return key{kind: keyUnknown}, 1
}

// decodeEscape handles the CSI sequences that the arrow, page and home/end
// keys send.
//
// An Esc that is not yet followed by anything is reported as incomplete
// rather than as the Esc key, because the rest of an arrow-key sequence may
// still be in flight. decodeFinal resolves it once that is ruled out.
func decodeEscape(buf []byte) (key, int, bool) {
	if len(buf) == 1 {
		return key{}, 0, false
	}
	if buf[1] != '[' && buf[1] != 'O' {
		// Alt-<something>. Not bound; drop both bytes.
		return key{kind: keyUnknown}, 2, true
	}
	if len(buf) < 3 {
		return key{}, 0, false
	}

	switch buf[2] {
	case 'A':
		return key{kind: keyUp}, 3, true
	case 'B':
		return key{kind: keyDown}, 3, true
	case 'C', 'D': // right, left: the query has no cursor to move
		return key{kind: keyUnknown}, 3, true
	case 'H':
		return key{kind: keyHome}, 3, true
	case 'F':
		return key{kind: keyEnd}, 3, true
	case '5', '6', '1', '4': // ESC [ n ~
		if len(buf) < 4 {
			return key{}, 0, false
		}
		if buf[3] != '~' {
			return key{kind: keyUnknown}, 4, true
		}
		switch buf[2] {
		case '5':
			return key{kind: keyPageUp}, 4, true
		case '6':
			return key{kind: keyPageDown}, 4, true
		case '1':
			return key{kind: keyHome}, 4, true
		default: // '4'
			return key{kind: keyEnd}, 4, true
		}
	}
	return key{kind: keyUnknown}, 3, true
}
