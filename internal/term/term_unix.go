//go:build darwin || dragonfly || freebsd || netbsd || openbsd || linux

package term

import (
	"syscall"
	"unsafe"
)

// termios is the terminal settings structure the kernel understands.
type termios = syscall.Termios

// winsize matches the kernel's struct winsize. The standard library does not
// export it for every unix, so nav declares it.
type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// ioctl issues a terminal control request against fd.
func ioctl(fd int, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

// IsTerminal reports whether fd refers to a terminal. Reading the terminal
// settings only succeeds for a real terminal, so that doubles as the test.
func IsTerminal(fd int) bool {
	var t termios
	return ioctl(fd, ioctlGetTermios, unsafe.Pointer(&t)) == nil
}

// MakeRaw switches fd into raw mode and returns the previous state. The
// caller must Restore that state before exiting, or the user's shell will be
// left without echo.
func MakeRaw(fd int) (*State, error) {
	var old termios
	if err := ioctl(fd, ioctlGetTermios, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}

	raw := old
	// Input: no CR->NL translation, no XON/XOFF flow control, no break
	// signal, no parity checks, no 8th-bit stripping.
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK |
		syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	// Output: no post-processing, so \n does not become \r\n behind our back.
	raw.Oflag &^= syscall.OPOST
	// Local: no echo, no line buffering, no signal generation (so Ctrl-C
	// arrives as a byte we can handle), no extended processing.
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON |
		syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	// Return from read as soon as one byte is available, with no timeout.
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := ioctl(fd, ioctlSetTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &State{termios: old}, nil
}

// Restore puts fd back into the state it was in before MakeRaw.
func Restore(fd int, state *State) error {
	if state == nil {
		return nil
	}
	return ioctl(fd, ioctlSetTermios, unsafe.Pointer(&state.termios))
}

// GetSize reports the character grid of the terminal on fd, falling back to
// FallbackSize when the terminal does not answer.
func GetSize(fd int) Size {
	var ws winsize
	if err := ioctl(fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return FallbackSize
	}
	if ws.rows == 0 || ws.cols == 0 {
		return FallbackSize
	}
	return Size{Rows: int(ws.rows), Cols: int(ws.cols)}
}
