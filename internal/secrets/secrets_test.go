package secrets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/secrets"
)

func cfgWithIdentity(identity string) config.Config {
	var cfg config.Config
	cfg.Secrets.Identity = identity
	return cfg
}

func cfgWithRecipient(recipient string) config.Config {
	var cfg config.Config
	cfg.Secrets.Recipient = recipient
	return cfg
}

// ── DecryptToMemory ───────────────────────────────────────────────────────────

func TestDecryptToMemory_NoIdentityFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "secret.age")
	os.WriteFile(src, []byte("x"), 0o600)
	missing := filepath.Join(t.TempDir(), "nonexistent-key.txt")

	cfg := cfgWithIdentity(missing)
	_, err := secrets.DecryptToMemory(src, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Failed to decrypt") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── Encrypt ───────────────────────────────────────────────────────────────────

func TestEncrypt_InvalidRecipient(t *testing.T) {
	src := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(src, []byte("my secret"), 0o600)

	cfg := cfgWithRecipient("not-a-valid-age-recipient")
	err := secrets.Encrypt(src, "", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Failed to encrypt") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEncrypt_NoRecipient(t *testing.T) {
	src := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(src, []byte("x"), 0o600)

	cfg := cfgWithRecipient("")
	err := secrets.Encrypt(src, "", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "No recipient") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── round-trip ────────────────────────────────────────────────────────────────

// genKeypair generates an age X25519 identity, writes it to a key file in the
// CLI-compatible format, and returns the file path and the public recipient.
func genKeypair(t *testing.T, dir string) (keyFile, recipient string) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	keyFile = filepath.Join(dir, "key.txt")
	contents := "# public key: " + id.Recipient().String() + "\n" + id.String() + "\n"
	if err := os.WriteFile(keyFile, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return keyFile, id.Recipient().String()
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	keyFile, recipient := genKeypair(t, dir)

	// Write plaintext and encrypt.
	plainSrc := filepath.Join(dir, "plain.txt")
	os.WriteFile(plainSrc, []byte("hello secrets"), 0o600)
	encDst := filepath.Join(dir, "plain.txt.age")

	encCfg := cfgWithRecipient(recipient)
	if err := secrets.Encrypt(plainSrc, encDst, encCfg); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt to memory.
	decCfg := cfgWithIdentity(keyFile)
	data, err := secrets.DecryptToMemory(encDst, decCfg)
	if err != nil {
		t.Fatalf("DecryptToMemory: %v", err)
	}
	if string(data) != "hello secrets" {
		t.Errorf("got %q, want %q", data, "hello secrets")
	}

	// Decrypt to file.
	decDst := filepath.Join(dir, "decrypted.txt")
	if err := secrets.Decrypt(encDst, decDst, decCfg); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	got, _ := os.ReadFile(decDst)
	if string(got) != "hello secrets" {
		t.Errorf("got %q, want %q", got, "hello secrets")
	}
}

// ── default dst paths ─────────────────────────────────────────────────────────

func TestEncrypt_DefaultDstPath(t *testing.T) {
	dir := t.TempDir()
	_, recipient := genKeypair(t, dir)

	src := filepath.Join(dir, "secret.txt")
	os.WriteFile(src, []byte("data"), 0o600)
	cfg := cfgWithRecipient(recipient)
	if err := secrets.Encrypt(src, "", cfg); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := os.Stat(src + ".age"); err != nil {
		t.Errorf("expected %s.age to exist: %v", src, err)
	}
}

func TestDecrypt_DefaultDstPath(t *testing.T) {
	dir := t.TempDir()
	keyFile, recipient := genKeypair(t, dir)

	// Encrypt first.
	src := filepath.Join(dir, "plain.txt")
	os.WriteFile(src, []byte("content"), 0o600)
	encCfg := cfgWithRecipient(recipient)
	if err := secrets.Encrypt(src, "", encCfg); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Now decrypt with no output path; should produce plain.txt.
	decCfg := cfgWithIdentity(keyFile)
	ageFile := src + ".age"
	if err := secrets.Decrypt(ageFile, "", decCfg); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	got, _ := os.ReadFile(src)
	if string(got) != "content" {
		t.Errorf("got %q, want \"content\"", got)
	}
}
