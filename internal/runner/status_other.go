//go:build !unix

package runner

import "os"

// interruptSignals are the signals nav ignores while a command runs. Only
// Interrupt is portable.
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// exitStatus reports the command's exit code. There is no portable way to
// recognise death by signal, so a process that did not exit normally is
// reported as ExitCode gives it, which is -1.
func exitStatus(state *os.ProcessState) int {
	if state == nil {
		return 0
	}
	return state.ExitCode()
}
