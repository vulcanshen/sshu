package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/vulcanshen/sshu/internal/store"
)

// fakeSSH answers handshakes on localhost with a throwaway ed25519 host key.
// It is the smallest server that can offer one, which is all ScanHostKey wants
// — the scan aborts before authentication, so nothing here needs to be real.
func fakeSSH(t *testing.T) (host string, port int, want ssh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return // the scan hangs up mid-handshake on purpose
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					for ch := range chans {
						ch.Reject(ssh.Prohibited, "not a real server")
					}
				}()
				sc.Wait()
			}()
		}
	}()

	h, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return h, n, signer.PublicKey()
}

func TestScanHostKeyBringsBackExactlyWhatTheServerOffered(t *testing.T) {
	host, port, want := fakeSSH(t)

	got, err := ScanHostKey(host, port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != want.Type() {
		t.Errorf("type = %q, want %q", got.Type, want.Type())
	}
	// The base64 must be the wire format, because that is the form a
	// known_hosts line holds — anything else would write a line ssh cannot read.
	if w := base64.StdEncoding.EncodeToString(want.Marshal()); got.Key != w {
		t.Errorf("key = %q, want %q", got.Key, w)
	}
	if w := ssh.FingerprintSHA256(want); got.Fingerprint != w {
		t.Errorf("fingerprint = %q, want %q", got.Fingerprint, w)
	}
}

// The fingerprint the user is shown before trusting a key, and the one the
// panel shows for that line afterwards, are computed by two different pieces of
// code — one through x/crypto/ssh, one from the stored base64. They have to
// agree, or the row would not look like the thing that was accepted.
func TestTheScannedFingerprintMatchesTheStoredOne(t *testing.T) {
	host, port, _ := fakeSSH(t)
	got, err := ScanHostKey(host, port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	stored := store.KnownHostEntry{Type: got.Type, Key: got.Key}.Fingerprint()
	if stored != got.Fingerprint {
		t.Errorf("store computed %q, the handshake said %q", stored, got.Fingerprint)
	}
}

func TestScanHostKeySaysWhichAddressWouldNotAnswer(t *testing.T) {
	// Port 1 on loopback: reliably refused, and refused fast.
	_, err := ScanHostKey("127.0.0.1", 1, 2*time.Second)
	if err == nil {
		t.Fatal("nothing is listening there, so this must fail")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("the error should name the address it tried, got %v", err)
	}
}
