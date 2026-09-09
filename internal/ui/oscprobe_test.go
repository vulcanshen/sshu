//go:build darwin || linux

package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// Probe, not a spec: can a nested sshu announce itself in its own output
// without the announcement showing up as garbage on screen? ptyquery_test
// covers the other direction — the emulator ANSWERING a child's query; this
// asks whether it stays quiet about a sequence it does not know.
//
// Every candidate carrier is printed between visible markers. An emulator that
// swallows them shows the markers and nothing else.
func TestTheEmulatorSwallowsAPrivateMarker(t *testing.T) {
	script := `printf '>>>1 ST\n'; printf '\033]7180;sshu;depth=2;locked=0\033\\'; ` +
		`printf '>>>2 BEL\n'; printf '\033]7180;sshu;depth=2;locked=0\007'; ` +
		`printf '>>>3 osc-alt\n'; printf '\033]777;sshu;depth=2\033\\'; ` +
		`printf '>>>4 DCS\n'; printf '\033Psshu;depth=2\033\\'; ` +
		`printf '>>>5 APC\n'; printf '\033_sshu;depth=2\033\\'; ` +
		`printf '>>>6 done\n'; sleep 30`

	p, err := startPty(exec.Command("sh", "-c", script), 60, 12)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.stop)

	waitFor(t, "the probe to finish printing", func() bool {
		return strings.Contains(strings.Join(p.screenLines(), "\n"), ">>>6 done")
	})

	screen := strings.Join(p.screenLines(), "\n")
	t.Logf("emulator screen:\n%s", screen)
	for _, leak := range []string{"7180", "777", "depth=2", "locked=0", "sshu;"} {
		if strings.Contains(screen, leak) {
			t.Errorf("the emulator PRINTED part of a private sequence (%q):\n%s", leak, screen)
		}
	}
}
