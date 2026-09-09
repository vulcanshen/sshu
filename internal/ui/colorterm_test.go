package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/vulcanshen/sshu/internal/store"
)

// ssh carries TERM by itself and nothing else, so a sshu on the far side sees
// 256 colours and quantises its whole UI. The depth has to be handed to it
// deliberately (§11.46).
func TestTheColourDepthIsHandedToTheFarSide(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	h := store.Host{Name: "h", Host: "example", Port: 22, User: "u"}

	var carried string
	for _, kv := range sshEnv(h, "") {
		if strings.HasPrefix(kv, colorEnv+"=") {
			carried = strings.TrimPrefix(kv, colorEnv+"=")
		}
	}
	if carried != "truecolor" {
		t.Errorf("the child's environment should carry the depth, got %q", carried)
	}

	// And ssh has to be TOLD to send it: SendEnv is not on by default in every
	// ssh_config, and a colour that depends on a file nobody edited is a colour
	// that is wrong on somebody's machine for no reason they can see.
	args := strings.Join(buildSSHCmd(h, "", 5).Args, " ")
	if !strings.Contains(args, "SendEnv="+colorEnv) {
		t.Errorf("ssh was not asked to send it: %s", args)
	}
}

// Nothing is carried when there is nothing to say. Sending an empty value
// would claim a depth the local terminal never reported.
func TestNothingIsCarriedWithoutALocalDepth(t *testing.T) {
	t.Setenv("COLORTERM", "")
	h := store.Host{Name: "h", Host: "example", Port: 22, User: "u"}
	for _, kv := range sshEnv(h, "") {
		if strings.HasPrefix(kv, colorEnv+"=") {
			t.Errorf("nothing should have been carried, got %q", kv)
		}
	}
	if args := strings.Join(buildSSHCmd(h, "", 5).Args, " "); strings.Contains(args, "SendEnv") {
		t.Errorf("ssh should not have been asked to send anything: %s", args)
	}
}

// The far side picks it up — but only into a gap. A real COLORTERM is the
// local terminal speaking for itself, and it outranks anything forwarded.
func TestTheFarSideAdoptsTheDepthOnlyIntoAGap(t *testing.T) {
	t.Run("adopted when missing", func(t *testing.T) {
		t.Setenv("COLORTERM", "")
		t.Setenv(colorEnv, "truecolor")
		AdoptForwardedColor()
		if got := os.Getenv("COLORTERM"); got != "truecolor" {
			t.Errorf("COLORTERM = %q, want the forwarded depth", got)
		}
	})
	t.Run("the local terminal wins", func(t *testing.T) {
		t.Setenv("COLORTERM", "24bit")
		t.Setenv(colorEnv, "truecolor")
		AdoptForwardedColor()
		if got := os.Getenv("COLORTERM"); got != "24bit" {
			t.Errorf("COLORTERM = %q, want the local terminal's own answer", got)
		}
	})
	t.Run("nothing invented from nothing", func(t *testing.T) {
		t.Setenv("COLORTERM", "")
		t.Setenv(colorEnv, "")
		AdoptForwardedColor()
		if got := os.Getenv("COLORTERM"); got != "" {
			t.Errorf("COLORTERM = %q, want it left alone", got)
		}
	})
}
