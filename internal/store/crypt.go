package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Passwords stop being plaintext on disk.
//
// What this buys, stated plainly, because the alternative is a feature that
// sounds safer than it is:
//
//	protects against  a hosts.yaml pasted into Slack, committed by accident,
//	                  or picked up on its own by a backup — the file is now
//	                  ciphertext and says nothing
//	does NOT protect  the whole config directory being synced or copied: the
//	                  key is in it. Nor anything else running as this user,
//	                  which can read the key exactly as sshu does.
//
// So this is SEPARATION, not confidentiality: what used to leak with one file
// now takes two. That is worth having — the first row is where secrets
// actually escape — but §8.3's warning stays, and the directory is still not
// something to sync.
//
// SSHU_KEY_FILE moves the key somewhere that is not copied with the config,
// which is the one way to cover the second row. It is deliberately not the
// default: a key outside the config directory is a key the user has to
// remember to back up, and losing it loses every password.
//
// Machine-bound derivation (a key computed from /etc/machine-id rather than
// stored) was considered and rejected. It would cover a config directory
// copied to ANOTHER machine, but /etc/machine-id changes every time a
// container is rebuilt — and sshu spends half its life inside containers and
// on remote Linux boxes — while a reinstall would lose every password with no
// way back.

// encPrefix marks an encrypted value. A password without it is plaintext and
// passes through untouched, which is what makes the upgrade from an older file
// work with no migration step: read the plaintext, write it back encrypted.
const encPrefix = "ENC:"

// keyFileName is the key's name inside the config directory.
//
// A dotfile, because it is the one thing in there that is NOT for reading or
// hand-editing. Everything else in the directory invites it — hosts.yaml says
// so in its own header — and a key sitting in the same listing reads as one
// more file to open and tidy. The leading dot is the directory saying which
// files are yours and which one is machinery.
const keyFileName = ".sshukey"

// KeyEnv names the environment variable that moves the key elsewhere.
const KeyEnv = "SSHU_KEY_FILE"

// keyBytes is AES-256. The nonce is what GCM asks for, and a fresh one is
// generated per encryption — reusing one with the same key is the single way
// to break GCM outright.
const (
	keyBytes   = 32
	nonceBytes = 12
)

// ErrNoKey is returned when a value needs decrypting and there is no key. It
// is distinguishable on purpose: "sshu cannot read this password" and "sshu
// cannot read this file" call for different words.
var ErrNoKey = errors.New("no key file")

// KeyPath is where the key lives.
func KeyPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(KeyEnv)); p != "" {
		return ExpandTilde(p), nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, keyFileName), nil
}

// EnsureKey creates the key if it is not there yet, and returns it either way.
//
// Called at startup so the file exists before anything needs it — a key that
// appears on the first save is a key the user has no reason to know about
// until they have already lost it.
func EnsureKey() ([]byte, error) {
	path, err := KeyPath()
	if err != nil {
		return nil, err
	}
	key, err := readKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrNoKey) {
		return nil, err
	}

	key = make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating a key: %w", err)
	}
	// base64 rather than raw bytes: the file is then a single line that can be
	// read, copied to another machine, and put in a password manager without
	// anything mangling it.
	body := base64.StdEncoding.EncodeToString(key) + "\n"
	if err := writeFile0600(path, []byte(body)); err != nil {
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	return key, nil
}

// loadKey reads the key without creating one. Decryption uses this: a missing
// key while ciphertext exists is a problem to report, not to paper over by
// inventing a new key that cannot open anything.
func loadKey() ([]byte, error) {
	path, err := KeyPath()
	if err != nil {
		return nil, err
	}
	return readKey(path)
}

func readKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s is not a sshu key file: %w", path, err)
	}
	if len(key) != keyBytes {
		return nil, fmt.Errorf("%s holds a %d-byte key, want %d", path, len(key), keyBytes)
	}
	return key, nil
}

// IsEncrypted reports whether a stored value is ciphertext.
func IsEncrypted(v string) bool { return strings.HasPrefix(v, encPrefix) }

// encryptSecret turns a plaintext password into its stored form.
//
// A value that is ALREADY encrypted passes straight through. That is what
// keeps a password whose ciphertext could not be opened — a key that was
// replaced, a file edited by hand — from being re-encrypted into nonsense or,
// far worse, written back as the empty string it decrypted to.
func encryptSecret(key []byte, plain string) (string, error) {
	if plain == "" || IsEncrypted(plain) {
		return plain, nil
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating a nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptSecret turns a stored value back into a password.
//
// A value without the prefix is plaintext — an older file, or one edited by
// hand — and comes back unchanged. On failure the value is returned AS IT WAS,
// still carrying its prefix, so the caller writing it back cannot destroy it.
func decryptSecret(key []byte, stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return stored, fmt.Errorf("not valid ciphertext: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return stored, err
	}
	if len(sealed) < nonceBytes {
		return stored, errors.New("ciphertext is too short to hold a nonce")
	}
	plain, err := gcm.Open(nil, sealed[:nonceBytes], sealed[nonceBytes:], nil)
	if err != nil {
		// GCM says only "message authentication failed", which is true and
		// unhelpful. The cause is almost always a different key.
		return stored, errors.New("could not be decrypted — this is not the key it was encrypted with")
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ---------------------------------------------------------------- documents

// anySecret reports whether there is anything here worth fetching a key for.
// A file of key-only hosts needs no key, and demanding one would make sshu
// refuse a config it can read perfectly well.
func anySecret(values []string) bool {
	for _, v := range values {
		if v != "" {
			return true
		}
	}
	return false
}

// anyEncrypted is the same question for reading: only a file that actually
// holds ciphertext needs the key to be present.
func anyEncrypted(values []string) bool {
	for _, v := range values {
		if IsEncrypted(v) {
			return true
		}
	}
	return false
}

// decryptInto opens every secret in place, and reports the names whose
// ciphertext would not open.
//
// Those keep their stored value untouched. sshu then runs with a password it
// cannot use — the connection will fail — but the ciphertext is still on disk,
// and putting the right key back recovers it. Blanking them would look tidier
// for one session and lose the password on the next save.
func decryptInto(names []string, secrets []*string) ([]string, error) {
	vals := make([]string, len(secrets))
	for i, s := range secrets {
		vals[i] = *s
	}
	if !anyEncrypted(vals) {
		return nil, nil
	}
	key, err := loadKey()
	if errors.Is(err, ErrNoKey) {
		// Ciphertext with no key at all. That is every sealed secret being
		// unreadable — news — and NOT a broken file. Returning an error here
		// stopped sshu from starting at all, which put the user outside the one
		// program that could show them what was wrong, while every host that
		// authenticates by key was perfectly fine.
		//
		// The secrets keep their stored values, so restoring the key file
		// recovers all of them.
		var bad []string
		for i, s := range secrets {
			if IsEncrypted(*s) {
				bad = append(bad, names[i])
			}
		}
		return bad, nil
	}
	if err != nil {
		// A key file that IS there and cannot be parsed is a different thing:
		// somebody put something else at that path, and quietly carrying on
		// would be the wrong answer to a question with an obvious fix.
		return nil, err
	}
	var bad []string
	for i, s := range secrets {
		plain, err := decryptSecret(key, *s)
		if err != nil {
			bad = append(bad, names[i])
			continue
		}
		*s = plain
	}
	return bad, nil
}

// encryptInto seals every secret in place. The key is created if it is not
// there: saving is the moment a password first needs one, and refusing then
// would mean a host that cannot be added.
func encryptInto(secrets []*string) error {
	vals := make([]string, len(secrets))
	for i, s := range secrets {
		vals[i] = *s
	}
	if !anySecret(vals) {
		return nil
	}
	key, err := EnsureKey()
	if err != nil {
		return err
	}
	for _, s := range secrets {
		sealed, err := encryptSecret(key, *s)
		if err != nil {
			return err
		}
		*s = sealed
	}
	return nil
}
