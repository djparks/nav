//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd || linux)

package term

// On platforms nav has no termios implementation for — Windows and Plan 9 —
// interactive selection is unavailable and the caller falls back to printing
// the matching cheats. The rest of nav still works.

// termios is a placeholder so State compiles everywhere.
type termios struct{}

// IsTerminal always reports false, which routes callers to the
// non-interactive path.
func IsTerminal(fd int) bool { return false }

// MakeRaw always fails on unsupported platforms.
func MakeRaw(fd int) (*State, error) { return nil, ErrUnsupported }

// Restore is a no-op on unsupported platforms.
func Restore(fd int, state *State) error { return nil }

// GetSize returns the fallback size on unsupported platforms.
func GetSize(fd int) Size { return FallbackSize }
