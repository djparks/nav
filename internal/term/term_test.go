package term

import (
	"os"
	"testing"
)

func TestIsTerminalOnARegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if IsTerminal(int(f.Fd())) {
		t.Error("IsTerminal returned true for a regular file")
	}
}

func TestMakeRawFailsOnARegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := MakeRaw(int(f.Fd())); err == nil {
		t.Error("MakeRaw succeeded on a regular file, want an error")
	}
}

func TestGetSizeFallsBack(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// A regular file has no window size, so the fallback applies.
	if got := GetSize(int(f.Fd())); got != FallbackSize {
		t.Errorf("GetSize = %+v, want the fallback %+v", got, FallbackSize)
	}
}

func TestRestoreWithNilStateIsANoOp(t *testing.T) {
	if err := Restore(0, nil); err != nil {
		t.Errorf("Restore(0, nil) = %v, want nil", err)
	}
}

// TestRealTerminalRoundTrip only runs when the test binary has a terminal,
// which is not the case under `go test` in CI. It is the one place the real
// raw-mode path gets exercised.
func TestRealTerminalRoundTrip(t *testing.T) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no controlling terminal")
	}
	defer tty.Close()

	fd := int(tty.Fd())
	if !IsTerminal(fd) {
		t.Skip("/dev/tty is not a terminal")
	}

	state, err := MakeRaw(fd)
	if err != nil {
		t.Fatalf("MakeRaw returned an error: %v", err)
	}
	if err := Restore(fd, state); err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	if size := GetSize(fd); size.Rows <= 0 || size.Cols <= 0 {
		t.Errorf("GetSize returned %+v, want positive dimensions", size)
	}
}
