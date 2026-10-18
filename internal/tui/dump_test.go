package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestDump prints screens to read by eye, without a terminal:
//
//	SSHU__DUMP="80x40:2,Enter,Space" go test ./internal/tui -run TestDump -v
//
// The value is the size, then the keys to press, comma-separated.
func TestDump(t *testing.T) {
	spec := os.Getenv("SSHU__DUMP")
	if spec == "" {
		t.Skip("set SSHU__DUMP to print a screen")
	}
	size, keys, _ := strings.Cut(spec, ":")
	ws, hs, _ := strings.Cut(size, "x")
	w, _ := strconv.Atoi(ws)
	h, _ := strconv.Atoi(hs)
	x := newHarness(t, w, h, testData(sampleHosts(), sampleCreds()))
	if keys != "" {
		for _, k := range strings.Split(keys, ",") {
			x.keys(k)
		}
	}
	fmt.Println(x.text())
}
