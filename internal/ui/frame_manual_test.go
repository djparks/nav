package ui

import (
	"os"
	"strings"
	"testing"

	"nav/internal/cheat"
	"nav/internal/term"
)

// TestDumpFrame is a manual aid: it prints a rendered frame so the layout can
// be eyeballed. Run with: go test ./internal/ui -run TestDumpFrame -v
func TestDumpFrame(t *testing.T) {
	if os.Getenv("NAV_DUMP_FRAME") == "" {
		t.Skip("set NAV_DUMP_FRAME=1 to print a frame")
	}
	cheats, err := cheat.LoadDir("../../cheats")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	Run(cheats, Options{
		Input:  strings.NewReader("docker\x1b[B\x1b"),
		Output: &out,
		Size:   func() term.Size { return term.Size{Rows: 16, Cols: 72} },
	})
	frames := strings.Split(out.String(), ansiHome)
	os.Stdout.WriteString("\n" + frames[len(frames)-1] + "\n")
}
