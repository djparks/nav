//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package term

import "syscall"

// The BSDs, macOS included, name the termios ioctls TIOCGETA/TIOCSETA.
const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)
