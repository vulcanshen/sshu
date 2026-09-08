package remote

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostKey is what a server offered when asked to identify itself: the three
// things a known_hosts line needs, plus the fingerprint a person compares.
type HostKey struct {
	Type        string
	Key         string // base64 — the wire format, exactly as known_hosts stores it
	Fingerprint string
}

// errEnough aborts the handshake once the key is in hand. It is not a failure:
// the key arrives BEFORE authentication, and going any further would mean
// offering credentials to a machine sshu has not decided to trust yet.
var errEnough = errors.New("host key collected")

// ScanHostKey opens a connection far enough to see the host key and no further.
//
// This is what `ssh-keyscan` does, and what ssh itself does on a first connect
// before it asks whether to trust the fingerprint. It exists because the
// alternative way to add a known host is to type 68 characters of base64 that
// somebody has to have got from somewhere else anyway — a form nobody would
// use, guarding a file that matters.
//
// Nothing is written here. The caller shows the fingerprint and asks.
func ScanHostKey(host string, port int, timeout time.Duration) (HostKey, error) {
	var got ssh.PublicKey
	cfg := &ssh.ClientConfig{
		// A user is required by the handshake and never reaches auth, so this
		// name only ever appears in the server's log — where saying who called
		// is more useful than "root".
		User: "sshu",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			got = key
			return errEnough
		},
		Timeout: timeout,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := ssh.Dial("tcp", addr, cfg)
	if conn != nil {
		conn.Close()
	}
	if got == nil {
		if err == nil {
			err = errors.New("no host key was offered")
		}
		return HostKey{}, fmt.Errorf("%s: %w", addr, err)
	}
	return HostKey{
		Type:        got.Type(),
		Key:         base64.StdEncoding.EncodeToString(got.Marshal()),
		Fingerprint: ssh.FingerprintSHA256(got),
	}, nil
}
