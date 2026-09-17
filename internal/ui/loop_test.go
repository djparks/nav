package ui

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// recorder is a minimal view that records the keys it is given and stops on
// Enter, Esc or Ctrl-C.
type recorder struct {
	got     []key
	renders int
}

func (r *recorder) handleKey(k key) action {
	r.got = append(r.got, k)
	switch k.kind {
	case keyEnter:
		return actionAccept
	case keyQuit:
		return actionCancel
	}
	return actionContinue
}

func (r *recorder) render() error {
	r.renders++
	return nil
}

// TestKeyReaderSpansViews is the regression test for a dropped-keystroke bug.
//
// A terminal delivers whatever has been typed in a single read, so one read
// can hold the Enter that finishes one view plus the keys meant for the next.
// Giving each view its own reader discarded those buffered bytes, which lost
// keystrokes whenever the user typed ahead or pasted.
func TestKeyReaderSpansViews(t *testing.T) {
	// All of this arrives in one read.
	kr := newKeyReader(strings.NewReader("a\rbc\r"))

	first := &recorder{}
	if act, err := runLoop(kr, first); act != actionAccept || err != nil {
		t.Fatalf("first view: act = %v, err = %v; want accept, nil", act, err)
	}
	if len(first.got) != 2 || first.got[0].r != 'a' {
		t.Fatalf("first view got %d keys, want 'a' then Enter", len(first.got))
	}

	// The bytes after the first Enter must still be available.
	second := &recorder{}
	if act, err := runLoop(kr, second); act != actionAccept || err != nil {
		t.Fatalf("second view: act = %v, err = %v; want accept, nil", act, err)
	}
	if len(second.got) != 3 {
		t.Fatalf("second view got %d keys, want 'b', 'c' and Enter", len(second.got))
	}
	if second.got[0].r != 'b' || second.got[1].r != 'c' {
		t.Errorf("second view got %q and %q, want 'b' and 'c'", second.got[0].r, second.got[1].r)
	}
}

func TestRunLoopRendersOncePerKeyPlusOnce(t *testing.T) {
	kr := newKeyReader(strings.NewReader("ab\r"))
	v := &recorder{}
	if _, err := runLoop(kr, v); err != nil {
		t.Fatalf("runLoop returned an error: %v", err)
	}
	// One initial frame, then one after each key that did not finish the
	// view: 'a' and 'b'. The Enter returns without redrawing.
	if v.renders != 3 {
		t.Errorf("rendered %d times, want 3", v.renders)
	}
}

func TestRunLoopEndOfInputCancels(t *testing.T) {
	kr := newKeyReader(strings.NewReader(""))
	act, err := runLoop(kr, &recorder{})
	if err != nil {
		t.Fatalf("runLoop returned an error: %v", err)
	}
	if act != actionCancel {
		t.Errorf("act = %v, want cancel", act)
	}
}

func TestRunLoopReportsReadErrors(t *testing.T) {
	kr := newKeyReader(errReader{})
	if _, err := runLoop(kr, &recorder{}); err == nil {
		t.Error("runLoop succeeded, want the read error to be reported")
	}
}

func TestRunLoopReportsRenderErrors(t *testing.T) {
	kr := newKeyReader(strings.NewReader("a"))
	if _, err := runLoop(kr, failingView{}); err == nil {
		t.Error("runLoop succeeded, want the render error to be reported")
	}
}

func TestKeyReaderWaitsForSplitSequences(t *testing.T) {
	// The arrow key arrives in pieces. next must wait for the rest rather
	// than read the lone Esc as a quit.
	kr := newKeyReader(&chunkedReader{chunks: []string{"\x1b", "[B"}})

	k, err := kr.next()
	if err != nil {
		t.Fatalf("next returned an error: %v", err)
	}
	if k.kind != keyDown {
		t.Errorf("kind = %d, want keyDown", k.kind)
	}
}

func TestKeyReaderResolvesLoneEscAtEndOfInput(t *testing.T) {
	kr := newKeyReader(strings.NewReader("\x1b"))

	k, err := kr.next()
	if err != nil {
		t.Fatalf("next returned an error: %v", err)
	}
	if k.kind != keyQuit {
		t.Errorf("kind = %d, want keyQuit", k.kind)
	}

	// The stream is exhausted, so the next call reports it.
	if _, err := kr.next(); !errors.Is(err, io.EOF) {
		t.Errorf("err = %v, want io.EOF", err)
	}
}

// errReader fails immediately with something other than EOF.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("input exploded") }

// failingView fails to draw.
type failingView struct{}

func (failingView) handleKey(key) action { return actionContinue }
func (failingView) render() error        { return errors.New("cannot draw") }
