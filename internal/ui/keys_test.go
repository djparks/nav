package ui

import "testing"

func TestDecodeControlKeys(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want keyKind
	}{
		{"ctrl-c quits", []byte{ctrlC}, keyQuit},
		{"carriage return selects", []byte{carriage}, keyEnter},
		{"line feed selects", []byte{lineFeed}, keyEnter},
		{"del is backspace", []byte{del}, keyBackspace},
		{"backspace is backspace", []byte{backspace}, keyBackspace},
		{"ctrl-y copies", []byte{ctrlY}, keyCopy},
		{"ctrl-u clears", []byte{ctrlU}, keyClearQuery},
		{"ctrl-w deletes a word", []byte{ctrlW}, keyDeleteWord},
		{"ctrl-p moves up", []byte{ctrlP}, keyUp},
		{"ctrl-n moves down", []byte{ctrlN}, keyDown},
		{"other control bytes are unknown", []byte{0x01}, keyUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, n, ok := decode(tt.in)
			if !ok {
				t.Fatal("decode reported an incomplete sequence")
			}
			if n != 1 {
				t.Errorf("consumed %d bytes, want 1", n)
			}
			if k.kind != tt.want {
				t.Errorf("kind = %d, want %d", k.kind, tt.want)
			}
		})
	}
}

func TestDecodeEscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		want     keyKind
		wantSize int
	}{
		{"up arrow", []byte("\x1b[A"), keyUp, 3},
		{"down arrow", []byte("\x1b[B"), keyDown, 3},
		{"application up", []byte("\x1bOA"), keyUp, 3},
		{"right arrow is unbound", []byte("\x1b[C"), keyUnknown, 3},
		{"left arrow is unbound", []byte("\x1b[D"), keyUnknown, 3},
		{"home", []byte("\x1b[H"), keyHome, 3},
		{"end", []byte("\x1b[F"), keyEnd, 3},
		{"page up", []byte("\x1b[5~"), keyPageUp, 4},
		{"page down", []byte("\x1b[6~"), keyPageDown, 4},
		{"home via tilde", []byte("\x1b[1~"), keyHome, 4},
		{"end via tilde", []byte("\x1b[4~"), keyEnd, 4},
		{"alt-x is unbound", []byte("\x1bx"), keyUnknown, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, n, ok := decode(tt.in)
			if !ok {
				t.Fatal("decode reported an incomplete sequence")
			}
			if k.kind != tt.want {
				t.Errorf("kind = %d, want %d", k.kind, tt.want)
			}
			if n != tt.wantSize {
				t.Errorf("consumed %d bytes, want %d", n, tt.wantSize)
			}
		})
	}
}

func TestDecodeIncompleteSequences(t *testing.T) {
	// An escape sequence split across reads must not be mistaken for a lone
	// Esc, which would quit the selector unexpectedly. A bare Esc is
	// reported as incomplete for the same reason: only decodeFinal, which
	// knows nothing more is coming, may turn it into a quit.
	for _, in := range [][]byte{[]byte("\x1b"), []byte("\x1b["), []byte("\x1b[5")} {
		if _, _, ok := decode(in); ok {
			t.Errorf("decode(%q) reported a complete key, want incomplete", in)
		}
	}
	if _, _, ok := decode(nil); ok {
		t.Error("decode(nil) reported a complete key, want incomplete")
	}
}

func TestDecodeFinalResolvesLeftovers(t *testing.T) {
	// An Esc with nothing after it really was the Esc key.
	k, n := decodeFinal([]byte{esc})
	if k.kind != keyQuit || n != 1 {
		t.Errorf("decodeFinal(esc) = %+v, %d; want quit consuming 1 byte", k, n)
	}

	// A sequence that began but never finished still starts with Esc, and
	// the leading Esc wins: quitting on Esc has to be dependable, and the
	// alternative would be to silently swallow the keypress.
	k, n = decodeFinal([]byte("\x1b["))
	if k.kind != keyQuit || n != 1 {
		t.Errorf("decodeFinal(\"\\x1b[\") = %+v, %d; want quit consuming 1 byte", k, n)
	}

	// A truncated multi-byte rune is dropped rather than guessed at.
	k, n = decodeFinal([]byte("é")[:1])
	if k.kind != keyUnknown || n != 1 {
		t.Errorf("decodeFinal(truncated rune) = %+v, %d; want unknown consuming 1 byte", k, n)
	}

	// Anything decode can already handle is passed straight through.
	k, n = decodeFinal([]byte("\x1b[A"))
	if k.kind != keyUp || n != 3 {
		t.Errorf("decodeFinal(up arrow) = %+v, %d; want up consuming 3 bytes", k, n)
	}

	if _, n := decodeFinal(nil); n != 0 {
		t.Errorf("decodeFinal(nil) consumed %d bytes, want 0", n)
	}
}

func TestDecodeRunes(t *testing.T) {
	k, n, ok := decode([]byte("a"))
	if !ok || k.kind != keyRune || k.r != 'a' || n != 1 {
		t.Errorf("decode(\"a\") = %+v, %d, %v; want rune 'a' consuming 1 byte", k, n, ok)
	}

	// A multi-byte rune must be decoded as one keypress, not several.
	k, n, ok = decode([]byte("é"))
	if !ok || k.kind != keyRune || k.r != 'é' || n != 2 {
		t.Errorf("decode(\"é\") = %+v, %d, %v; want rune 'é' consuming 2 bytes", k, n, ok)
	}

	// The leading byte of a two-byte rune on its own is incomplete.
	if _, _, ok := decode([]byte("é")[:1]); ok {
		t.Error("decode of a truncated rune reported a complete key, want incomplete")
	}
}

func TestDecodeSequenceOfKeys(t *testing.T) {
	// Several keypresses can arrive in one read; decode must peel them off
	// one at a time.
	buf := []byte("hi\x1b[B\r")
	want := []keyKind{keyRune, keyRune, keyDown, keyEnter}

	for i, wantKind := range want {
		k, n, ok := decode(buf)
		if !ok {
			t.Fatalf("key %d: decode reported an incomplete sequence", i)
		}
		if k.kind != wantKind {
			t.Errorf("key %d: kind = %d, want %d", i, k.kind, wantKind)
		}
		buf = buf[n:]
	}
	if len(buf) != 0 {
		t.Errorf("%d bytes left over, want 0", len(buf))
	}
}
