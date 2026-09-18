//go:build unix

package runner

import (
	"os"
	"syscall"
)

// interruptSignals are the signals nav ignores while a command runs, leaving
// them to the command that now owns the terminal.
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGQUIT}
}

// exitStatus turns a finished process into the status a shell would report.
//
// A command killed by a signal has no exit code of its own, so the shell
// convention of 128 plus the signal number is used: Ctrl-C becomes 130, the
// same value the user's shell would report.
func exitStatus(state *os.ProcessState) int {
	if state == nil {
		return 0
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}
