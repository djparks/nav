// Package clipboard copies text to the system clipboard.
//
// There is no portable system call for this, so nav shells out to whichever
// of the usual helper programs is installed. That keeps the package free of
// dependencies and of cgo.
package clipboard

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoTool is returned when none of the known clipboard programs is
// installed. Callers should treat this as "copying is unavailable" rather
// than as a fatal error.
var ErrNoTool = errors.New("no clipboard program found")

// tool is one candidate clipboard program.
type tool struct {
	name string
	args []string
}

// tools lists the candidates for the current platform, best first.
func tools() []tool {
	switch runtime.GOOS {
	case "darwin":
		return []tool{{name: "pbcopy"}}
	case "windows":
		return []tool{{name: "clip"}}
	default:
		// Wayland first, then the two common X11 helpers, then termux.
		return []tool{
			{name: "wl-copy"},
			{name: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "xsel", args: []string{"--clipboard", "--input"}},
			{name: "termux-clipboard-set"},
		}
	}
}

// Available reports whether a clipboard program could be found, so a caller
// can hide a "copy" hint when copying will not work.
func Available() bool {
	_, err := find()
	return err == nil
}

// find returns the first clipboard tool present on PATH.
func find() (tool, error) {
	for _, t := range tools() {
		if _, err := exec.LookPath(t.name); err == nil {
			return t, nil
		}
	}
	return tool{}, ErrNoTool
}

// Copy writes text to the system clipboard.
func Copy(text string) error {
	t, err := find()
	if err != nil {
		return err
	}

	cmd := exec.Command(t.name, t.args...)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s: %w: %s", t.name, err, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("%s: %w", t.name, err)
	}
	return nil
}
