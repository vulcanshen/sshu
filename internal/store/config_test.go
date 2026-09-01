package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A missing config.yaml is the normal case, not an error: every setting has a
// default that is what sshu did before the setting existed.
func TestNoConfigIsNotAProblem(t *testing.T) {
	t.Setenv("SSHU_CONFIG", t.TempDir())

	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("a missing file should not be an error: %v", err)
	}
	if c.Timeout() != DefaultConnectTimeout*time.Second {
		t.Errorf("timeout is %v, want the default", c.Timeout())
	}
}

func TestConfigIsRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSHU_CONFIG", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("# how long one connection attempt gets\nconnect_timeout: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Timeout() != 4*time.Second {
		t.Errorf("timeout is %v, want 4s", c.Timeout())
	}
	if c.Seconds() != 4 {
		t.Errorf("seconds is %d, want 4", c.Seconds())
	}
}

// A number outside the sane range is a slipped decimal, not an intention. It is
// replaced rather than obeyed, because obeying "connect_timeout: 0" would mean
// a tab that never connects and never says why.
func TestAnAbsurdTimeoutFallsBackToTheDefault(t *testing.T) {
	for _, n := range []int{0, -30, 999999} {
		c := Config{ConnectTimeout: n}
		if c.Timeout() != DefaultConnectTimeout*time.Second {
			t.Errorf("connect_timeout: %d gave %v, want the default", n, c.Timeout())
		}
	}
	// And the ends of the range ARE honoured — clamping must not eat the values
	// somebody would reasonably pick.
	for _, n := range []int{1, 600} {
		if c := (Config{ConnectTimeout: n}); c.Seconds() != n {
			t.Errorf("connect_timeout: %d gave %d", n, c.Seconds())
		}
	}
}

// A file that cannot be parsed is reported AND survivable: sshu starts on the
// defaults, and the caller decides how to tell somebody.
func TestABrokenConfigIsReportedButNotFatal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSHU_CONFIG", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("connect_timeout: [this is not a number\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := LoadConfig()
	if err == nil {
		t.Error("a malformed file should be reported")
	}
	if c.Timeout() != DefaultConnectTimeout*time.Second {
		t.Errorf("timeout is %v, want the default to still be usable", c.Timeout())
	}
}
