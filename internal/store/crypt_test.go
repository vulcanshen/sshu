package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain points the key file at a temporary directory for the whole package.
//
// Without it, every test that saves a host creates a key in the USER'S config
// directory: KeyPath falls back to store.Dir(), and writing the yaml itself to
// a t.TempDir() does nothing to change that. It happened once, which is why
// this is here rather than left to each test to remember.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sshu-store")
	if err != nil {
		panic(err)
	}
	os.Setenv(KeyEnv, filepath.Join(dir, keyFileName))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// keyIn points this test at a key of its own, so one test replacing a key
// cannot decide what another test can open.
func keyIn(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), keyFileName)
	t.Setenv(KeyEnv, p)
	return p
}

// --------------------------------------------------------------------- keys

// The key is created on first use, at 0600, as one line of base64 — a file
// that can be read, copied to another machine and pasted into a password
// manager without anything mangling it.
func TestTheKeyIsCreatedOnceAtTheRightMode(t *testing.T) {
	p := keyIn(t)

	first, err := EnsureKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != keyBytes {
		t.Fatalf("want a %d-byte key, got %d", keyBytes, len(first))
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Errorf("the key must be 0600, got %o", perm)
	}
	raw, _ := os.ReadFile(p)
	if strings.Count(strings.TrimSpace(string(raw)), "\n") != 0 {
		t.Errorf("the key should be one line: %q", raw)
	}

	// Called again, it does NOT make a new one — a second key would be a
	// silent way to lose every password already sealed with the first.
	again, err := EnsureKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(first) {
		t.Error("EnsureKey generated a second key over the first")
	}
}

// The environment variable moves the key, which is the one way to keep it out
// of a config directory that gets synced.
func TestTheKeyFileCanBeMovedByEnvironment(t *testing.T) {
	elsewhere := filepath.Join(t.TempDir(), "somewhere", "my.key")
	t.Setenv(KeyEnv, elsewhere)

	if _, err := EnsureKey(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(elsewhere); err != nil {
		t.Errorf("the key should be where the environment said: %v", err)
	}
	if got, _ := KeyPath(); got != elsewhere {
		t.Errorf("KeyPath is %q, want %q", got, elsewhere)
	}
}

func TestAKeyFileThatIsNotAKeySaysSo(t *testing.T) {
	p := keyIn(t)
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte("this is not base64!!!\n"), 0o600)

	if _, err := EnsureKey(); err == nil {
		t.Error("a corrupt key file must be reported, not replaced")
	}
	// ...and it is NOT overwritten. Replacing it would destroy the only thing
	// that can open the passwords, on the strength of a parse error.
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "not base64") {
		t.Errorf("the key file was overwritten: %q", raw)
	}
}

// -------------------------------------------------------------- the cipher

func TestASecretSurvivesTheRoundTrip(t *testing.T) {
	keyIn(t)
	key, err := EnsureKey()
	if err != nil {
		t.Fatal(err)
	}
	const plain = "hunter2 · 中文 · \n newline"

	sealed, err := encryptSecret(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, encPrefix) {
		t.Errorf("a sealed value must be marked: %q", sealed)
	}
	if strings.Contains(sealed, "hunter2") {
		t.Fatalf("the password is still readable: %q", sealed)
	}
	got, err := decryptSecret(key, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("round trip gave %q, want %q", got, plain)
	}
}

// A fresh nonce every time. Reusing one under the same key is the single way
// to break GCM outright, and identical ciphertext for identical passwords
// would leak which hosts share one.
func TestTheSameSecretSealsDifferentlyEachTime(t *testing.T) {
	keyIn(t)
	key, _ := EnsureKey()
	a, _ := encryptSecret(key, "same")
	b, _ := encryptSecret(key, "same")
	if a == b {
		t.Error("two seals of one password came out identical — the nonce is not fresh")
	}
}

// Plaintext passes through untouched, in both directions. That is what lets an
// older file be read with no migration step and a hand-edited one keep working.
func TestPlaintextPassesThroughUnchanged(t *testing.T) {
	keyIn(t)
	key, _ := EnsureKey()
	if got, err := decryptSecret(key, "hunter2"); err != nil || got != "hunter2" {
		t.Errorf("a plaintext value should come back as-is, got %q (%v)", got, err)
	}
	if got, _ := encryptSecret(key, ""); got != "" {
		t.Errorf("an empty password should stay empty, got %q", got)
	}
}

// Sealing something already sealed would produce ciphertext of ciphertext —
// and, worse, would be the path by which a value that failed to open gets
// rewritten as something nobody can ever open.
func TestAlreadySealedIsNotSealedAgain(t *testing.T) {
	keyIn(t)
	key, _ := EnsureKey()
	once, _ := encryptSecret(key, "hunter2")
	twice, _ := encryptSecret(key, once)
	if twice != once {
		t.Errorf("double sealing changed the value:\n once %q\ntwice %q", once, twice)
	}
}

// The wrong key fails, says something a person can act on, and — the part that
// matters — hands the value BACK UNCHANGED so a caller writing it out cannot
// destroy it.
func TestTheWrongKeyFailsWithoutDestroyingAnything(t *testing.T) {
	keyIn(t)
	key, _ := EnsureKey()
	sealed, _ := encryptSecret(key, "hunter2")

	other := make([]byte, keyBytes)
	for i := range other {
		other[i] = byte(i)
	}
	got, err := decryptSecret(other, sealed)
	if err == nil {
		t.Fatal("the wrong key must not open a secret")
	}
	if got != sealed {
		t.Errorf("the ciphertext must come back untouched:\n got %q\nwant %q", got, sealed)
	}
	if !strings.Contains(err.Error(), "not the key") {
		t.Errorf("the message should name the likely cause, got %q", err)
	}
}

// ------------------------------------------------------------- the documents

// The whole point, checked on the bytes: a saved hosts.yaml does not contain
// the password.
func TestASavedFileDoesNotContainThePassword(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	in := File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "s3cr3t"},
	}}
	if err := SaveTo(p, in); err != nil {
		t.Fatal(err)
	}
	raw := mustRead(t, p)
	if strings.Contains(raw, "s3cr3t") {
		t.Fatalf("the password is in the file:\n%s", raw)
	}
	if !strings.Contains(raw, encPrefix) {
		t.Errorf("the password should be marked as sealed:\n%s", raw)
	}

	out, _, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Hosts[0].Password != "s3cr3t" {
		t.Errorf("loading should give the password back, got %q", out.Hosts[0].Password)
	}
}

// Saving must not change the caller's data. The UI builds a File from the very
// slice it is displaying, so sealing in place would swap the passwords on
// screen for ciphertext — and the next edit would save that as though it were
// what the user typed.
func TestSavingDoesNotSealTheCallersCopy(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	hosts := []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "s3cr3t"},
	}
	if err := SaveTo(p, File{Hosts: hosts}); err != nil {
		t.Fatal(err)
	}
	if hosts[0].Password != "s3cr3t" {
		t.Errorf("the caller's host was sealed in place: %q", hosts[0].Password)
	}

	// Same for credentials.
	cp := filepath.Join(t.TempDir(), "credentials.yaml")
	creds := []Credential{{Name: "ops", User: "ops", Auth: AuthPassword, Password: "s3cr3t"}}
	if err := SaveCredsTo(cp, CredsFile{Credentials: creds}); err != nil {
		t.Fatal(err)
	}
	if creds[0].Password != "s3cr3t" {
		t.Errorf("the caller's credential was sealed in place: %q", creds[0].Password)
	}
}

// A plaintext password reads fine and is sealed on the next save. No migration
// code and no version bump: the prefix is what tells the two apart.
//
// The file here is at the CURRENT version on purpose. Encryption deliberately
// did not move the version number, so "needs sealing" cannot be inferred from
// it — HasPlaintextSecret is the question, and this is the case that proves it
// has to be asked separately.
func TestAPlaintextPasswordIsSealedOnItsNextSave(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(p, []byte(`version: 2
hosts:
  - name: web
    host: h
    port: 22
    user: u
    auth: password
    password: "s3cr3t"
`), 0o600)

	out, _, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Hosts[0].Password != "s3cr3t" {
		t.Fatalf("a plaintext password should read straight through, got %q", out.Hosts[0].Password)
	}
	if out.NeedsUpgrade() {
		t.Error("this file is at the current version — encryption did not move it")
	}
	if !out.HadPlaintextSecret {
		t.Fatal("a file holding a plaintext password must say so")
	}
	if err := SaveTo(p, out); err != nil {
		t.Fatal(err)
	}
	if raw := mustRead(t, p); strings.Contains(raw, "s3cr3t") {
		t.Errorf("saving left the password in the clear:\n%s", raw)
	}

	// ...and once sealed it stops asking, or startup would rewrite the file on
	// every single run. This is the assertion that caught the first version,
	// which asked the loaded struct — where the password is plaintext by
	// definition, because loading is what decrypts it.
	sealedFile, _, _ := LoadFrom(p)
	if sealedFile.HadPlaintextSecret {
		t.Error("a sealed file must not keep asking to be rewritten")
	}
	if sealedFile.Hosts[0].Password != "s3cr3t" {
		t.Errorf("...while still reading back as the password: %q", sealedFile.Hosts[0].Password)
	}
}

// A password that will not open is REPORTED and KEPT. Blanking it would look
// tidier for one session and lose the secret on the next save; this way the
// ciphertext survives for the right key to open later.
func TestAnUnreadableSecretIsReportedAndKept(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	if err := SaveTo(p, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "s3cr3t"},
	}}); err != nil {
		t.Fatal(err)
	}
	sealed := mustRead(t, p)

	// A different key: the same situation as restoring a config without its
	// key file.
	keyIn(t)
	if _, err := EnsureKey(); err != nil {
		t.Fatal(err)
	}

	out, _, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("an unreadable password must not stop the file loading: %v", err)
	}
	if len(out.UnreadableSecrets) != 1 || out.UnreadableSecrets[0] != "web" {
		t.Errorf("want the host named, got %v", out.UnreadableSecrets)
	}
	if !IsEncrypted(out.Hosts[0].Password) {
		t.Errorf("the ciphertext must be kept, got %q", out.Hosts[0].Password)
	}

	// And saving it back does not destroy it.
	if err := SaveTo(p, out); err != nil {
		t.Fatal(err)
	}
	if after := mustRead(t, p); secretLine(after) != secretLine(sealed) {
		t.Errorf("the secret changed after a save it should not have touched:\nbefore %s\nafter  %s",
			secretLine(sealed), secretLine(after))
	}
}

// A file with no passwords in it needs no key at all — demanding one would
// make sshu refuse a config of key-only hosts that it can read perfectly well.
func TestAFileWithoutSecretsNeedsNoKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(p, []byte(`version: 2
hosts:
  - name: web
    host: h
    port: 22
    user: u
    auth: privatekey
    identity_file: ~/.ssh/id_ed25519
`), 0o600)

	// No key anywhere.
	t.Setenv(KeyEnv, filepath.Join(t.TempDir(), "absent", keyFileName))
	if _, _, err := LoadFrom(p); err != nil {
		t.Errorf("a file with nothing to decrypt should load without a key: %v", err)
	}
}

// Ciphertext with no key at all still LOADS. Every sealed secret is
// unreadable, which is news and not a broken file — and refusing would put the
// user outside the one program that can show them what happened, while every
// host that authenticates by key is perfectly fine.
//
// The first version of this returned an error, and the app would not start.
func TestCiphertextWithoutAKeyLoadsAndReports(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	if err := SaveTo(p, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "s3cr3t"},
		{Name: "keyonly", Host: "h", Port: 22, User: "u", Auth: AuthPrivateKey,
			IdentityFile: "~/.ssh/id_ed25519"},
	}}); err != nil {
		t.Fatal(err)
	}

	t.Setenv(KeyEnv, filepath.Join(t.TempDir(), "gone", keyFileName))
	out, _, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("a missing key must not stop the file loading: %v", err)
	}
	if len(out.UnreadableSecrets) != 1 || out.UnreadableSecrets[0] != "web" {
		t.Errorf("want the one sealed host named, got %v", out.UnreadableSecrets)
	}
	if !IsEncrypted(out.Hosts[0].Password) {
		t.Errorf("the ciphertext must be kept for the key to come back to: %q", out.Hosts[0].Password)
	}
	if out.Hosts[1].IdentityFile == "" {
		t.Error("a key-authenticated host beside it should be untouched")
	}
}

// A key file that IS there and is not a key is a different thing: somebody put
// something else at that path, and carrying on quietly would be the wrong
// answer to a question with an obvious fix.
func TestAKeyFileThatIsGarbageStopsTheLoad(t *testing.T) {
	keyIn(t)
	p := filepath.Join(t.TempDir(), "hosts.yaml")
	if err := SaveTo(p, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "s3cr3t"},
	}}); err != nil {
		t.Fatal(err)
	}

	kp, _ := KeyPath()
	os.WriteFile(kp, []byte("not a key at all !!!\n"), 0o600)
	if _, _, err := LoadFrom(p); err == nil {
		t.Error("a key file that is not a key should be reported, not worked around")
	}
}

// secretLine pulls the password line out of a document, so a comparison says
// which line differed rather than dumping the whole file.
func secretLine(doc string) string {
	for _, l := range strings.Split(doc, "\n") {
		if strings.Contains(l, "password:") {
			return strings.TrimSpace(l)
		}
	}
	return "(no password line)"
}
