package secrets_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

func TestDecryptToMemory_NoAgeBinary(t *testing.T) {
	// Arrange: point PATH to an empty dir so age is not found.
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	src := filepath.Join(t.TempDir(), "secret.age")
	os.WriteFile(src, []byte("x"), 0o600)
	identity := filepath.Join(t.TempDir(), "key.txt")
	os.WriteFile(identity, []byte("AGE-SECRET-KEY-1..."), 0o600)

	cfg := cfgWithIdentity(identity)
	_, err := secrets.DecryptToMemory(src, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "age") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDecryptToMemory_NoIdentityFile(t *testing.T) {
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not on PATH")
	}

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

func TestEncrypt_NoAgeBinary(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	src := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(src, []byte("my secret"), 0o600)

	cfg := cfgWithRecipient("age1abc...")
	err := secrets.Encrypt(src, "", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "age") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEncrypt_NoRecipient(t *testing.T) {
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not on PATH")
	}

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

// ── round-trip (skipped unless age is present and not in short mode) ──────────

func TestRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping round-trip test in short mode")
	}
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not on PATH")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not on PATH")
	}

	dir := t.TempDir()

	// Generate a keypair.
	keyFile := filepath.Join(dir, "key.txt")
	out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}

	// Extract the public key from the generated file.
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	recipient := ""
	for _, line := range strings.Split(string(keyBytes), "\n") {
		if strings.HasPrefix(line, "# public key:") {
			recipient = strings.TrimSpace(strings.TrimPrefix(line, "# public key:"))
			break
		}
	}
	if recipient == "" {
		t.Fatal("could not parse public key from age-keygen output")
	}

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
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not on PATH")
	}
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not on PATH")
	}

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.txt")
	out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}
	keyBytes, _ := os.ReadFile(keyFile)
	recipient := ""
	for _, line := range strings.Split(string(keyBytes), "\n") {
		if strings.HasPrefix(line, "# public key:") {
			recipient = strings.TrimSpace(strings.TrimPrefix(line, "# public key:"))
			break
		}
	}

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
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not on PATH")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not on PATH")
	}

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.txt")
	out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}
	keyBytes, _ := os.ReadFile(keyFile)
	recipient := ""
	for _, line := range strings.Split(string(keyBytes), "\n") {
		if strings.HasPrefix(line, "# public key:") {
			recipient = strings.TrimSpace(strings.TrimPrefix(line, "# public key:"))
			break
		}
	}

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
