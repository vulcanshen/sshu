package store

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// config.yaml holds sshu's settings, as opposed to hosts.yaml which holds its
// data. It is READ-ONLY from sshu's side: nothing in the UI writes it back, so
// a hand-edited file is never reformatted, re-ordered or clobbered, and the
// comments a person put in it survive.
//
// A missing file is not an error. Every setting has a default that is what sshu
// did before the setting existed, so the file only has to say what you want
// changed.

// DefaultConnectTimeout is how long one connection attempt gets. Fifteen
// seconds is what both tabs used as a constant before this was configurable.
const DefaultConnectTimeout = 15

// Connect timeouts outside this range are treated as a typo rather than an
// intention. One second is already aggressive for a link with any latency, and
// ten minutes is long past the point where a person has concluded the app is
// broken — a value beyond either end is more likely a slipped decimal than a
// choice.
const (
	minConnectTimeout = 1
	maxConnectTimeout = 600
)

// Config is the whole of config.yaml.
type Config struct {
	// ConnectTimeout bounds ONE connection attempt, in seconds. It reaches both
	// tabs: tab [3] passes it to ssh as -o ConnectTimeout, and tab [2] uses it
	// as the SSH client's dial timeout.
	ConnectTimeout int `yaml:"connect_timeout"`
}

// DefaultConfig is what sshu runs as when there is no file.
func DefaultConfig() Config {
	return Config{ConnectTimeout: DefaultConnectTimeout}
}

// Timeout is ConnectTimeout as a duration, with anything out of range replaced
// by the default rather than obeyed.
func (c Config) Timeout() time.Duration {
	n := c.ConnectTimeout
	if n < minConnectTimeout || n > maxConnectTimeout {
		n = DefaultConnectTimeout
	}
	return time.Duration(n) * time.Second
}

// Seconds is Timeout in whole seconds, for the `-o ConnectTimeout=N` that ssh
// wants as a number.
func (c Config) Seconds() int { return int(c.Timeout() / time.Second) }

// ConfigPath is the full path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// LoadConfig reads config.yaml, or returns the defaults when there is none.
//
// A file that cannot be PARSED is a different matter from a file that is not
// there: the first means somebody wrote something and it is not being honoured,
// which they need to hear about. The defaults are returned either way, so a
// broken settings file never stops sshu from starting.
func LoadConfig() (Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	c := DefaultConfig()
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return DefaultConfig(), err
	}
	return c, nil
}
