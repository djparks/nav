package ui

import (
	"errors"
	"io"
	"time"
)

// escDelay is how long the reader waits for the rest of an escape sequence
// before concluding that the user pressed Esc on its own.
//
// Pressing Esc and pressing the up arrow both start with the same byte. The
// only thing that distinguishes them is that the arrow's remaining bytes
// follow immediately, so telling them apart needs a short wait. Terminals
// send the whole sequence in one burst, and no human can type a second key
// this fast, which makes the wait invisible in practice.
var escDelay = 50 * time.Millisecond

// action is what the event loop should do after a view handles a key.
type action int

const (
	actionContinue action = iota // keep going
	actionAccept                 // the user committed: Enter
	actionCancel                 // the user backed out: Esc or Ctrl-C
)

// view is one interactive screen: the cheat selector, or a prompt for a
// single variable. Keeping this interface tiny is what lets both of them
// share the input handling in runLoop.
type view interface {
	// handleKey applies a keypress and says whether the loop should stop.
	handleKey(k key) action
	// render draws the current state.
	render() error
}

// readResult is one delivery from the input-reading goroutine.
type readResult struct {
	data []byte
	err  error
}

// keyReader turns a byte stream into keypresses.
//
// One keyReader must serve a whole session — the cheat list and every
// variable prompt after it. A terminal hands over whatever has been typed in
// one read, so a single read can easily contain the Enter that finishes one
// screen plus the first keystrokes meant for the next. Those extra bytes sit
// in pending, and creating a second reader partway through would throw them
// away: typing ahead, or pasting, would silently lose keys.
type keyReader struct {
	reads    chan readResult
	pending  []byte
	inputErr error
}

// newKeyReader starts reading r in the background.
//
// The goroutine ends when r reports an error, which happens when the caller
// closes the terminal once the session is over.
func newKeyReader(r io.Reader) *keyReader {
	kr := &keyReader{reads: make(chan readResult, 4)}
	go readInput(r, kr.reads)
	return kr
}

// next blocks until a whole keypress is available.
//
// It returns io.EOF once the input has closed and every buffered byte has
// been handed out.
func (kr *keyReader) next() (key, error) {
	for {
		// The buffered bytes may already hold a complete keypress.
		if k, used, ok := decode(kr.pending); ok {
			kr.pending = kr.pending[used:]
			return k, nil
		}

		// pending is now either empty or an incomplete sequence.
		if kr.inputErr != nil {
			if len(kr.pending) > 0 {
				// Nothing more is coming, so take the leftover at face value.
				k, used := decodeFinal(kr.pending)
				kr.pending = kr.pending[used:]
				return k, nil
			}
			return key{}, kr.inputErr
		}

		var res readResult
		if len(kr.pending) > 0 {
			// Waiting on the rest of a sequence: give up after escDelay and
			// take the bytes at face value.
			timer := time.NewTimer(escDelay)
			select {
			case res = <-kr.reads:
				timer.Stop()
			case <-timer.C:
				k, used := decodeFinal(kr.pending)
				kr.pending = kr.pending[used:]
				return k, nil
			}
		} else {
			res = <-kr.reads
		}

		kr.pending = append(kr.pending, res.data...)
		if res.err != nil {
			kr.inputErr = res.err
		}
	}
}

// runLoop draws v and feeds it keypresses until it accepts or cancels.
//
// Running out of input — a closed pipe, or the end of a scripted test
// input — counts as cancelling, so the loop always terminates.
func runLoop(kr *keyReader, v view) (action, error) {
	if err := v.render(); err != nil {
		return actionCancel, err
	}

	for {
		k, err := kr.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return actionCancel, nil
			}
			return actionCancel, err
		}

		if act := v.handleKey(k); act != actionContinue {
			return act, nil
		}
		if err := v.render(); err != nil {
			return actionCancel, err
		}
	}
}

// readInput copies keypress bytes from r onto ch until r fails.
func readInput(r io.Reader, ch chan<- readResult) {
	for {
		buf := make([]byte, 64)
		n, err := r.Read(buf)
		if n > 0 {
			ch <- readResult{data: buf[:n]}
		}
		if err != nil {
			ch <- readResult{err: err}
			return
		}
	}
}
