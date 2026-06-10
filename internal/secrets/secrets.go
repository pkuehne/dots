// Package secrets wraps age for encrypting and decrypting .age files.
package secrets

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
)

// Encrypt encrypts src using cfg.Secrets.Recipient and writes to dst.
// If dst is empty, dst = src + ".age".
func Encrypt(src, dst string, cfg config.Config) error {
	if _, err := exec.LookPath("age"); err != nil {
		return &errs.DotsError{
			Msg:  "Cannot encrypt — 'age' not found on PATH",
			Hint: "Install age:\n  https://github.com/FiloSottile/age/releases",
		}
	}
	if cfg.Secrets.Recipient == "" {
		return &errs.DotsError{
			Msg:  "No recipient configured for encryption",
			Hint: "Set in dots.toml:\n  [secrets]\n  recipient = \"age1...\"",
		}
	}
	out := dst
	if out == "" {
		out = src + ".age"
	}
	cmd := exec.Command("age", "--encrypt", "-r", cfg.Secrets.Recipient, "-o", out, src)
	if output, err := cmd.CombinedOutput(); err != nil {
		stderr := strings.TrimSpace(string(output))
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %s", stderr),
		}
	}
	return nil
}

// Decrypt decrypts src using cfg.Secrets.Identity and writes to dst.
// If dst is empty, dst is src with the ".age" suffix removed.
func Decrypt(src, dst string, cfg config.Config) error {
	data, err := DecryptToMemory(src, cfg)
	if err != nil {
		return err
	}
	out := dst
	if out == "" {
		out = strings.TrimSuffix(src, ".age")
	}
	if err := fileutil.EnsureParent(out); err != nil {
		return err
	}
	return writeFile(out, data)
}

// DecryptToMemory returns the plaintext of an .age file without writing to disk.
// Used during apply to render decrypted files in-memory.
func DecryptToMemory(src string, cfg config.Config) ([]byte, error) {
	if _, err := exec.LookPath("age"); err != nil {
		return nil, &errs.DotsError{
			Msg: fmt.Sprintf("Cannot decrypt %s — 'age' not found on PATH", baseName(src)),
			Hint: "Install age first:\n  dots tools install age\n" +
				"Or download from: https://github.com/FiloSottile/age/releases",
		}
	}
	identity := fileutil.Expand(cfg.Secrets.Identity)
	if !fileExists(identity) {
		return nil, &errs.DotsError{
			Msg: fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: age identity file not found: %s\n\n"+
				"Hint: Generate an age keypair with:\n"+
				"  age-keygen -o %s\n"+
				"Then set the public key as recipient in dots.toml:\n"+
				"  [secrets]\n"+
				"  recipient = \"age1...\"",
				identity, identity),
		}
	}
	cmd := exec.Command("age", "--decrypt", "-i", identity, src)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr == "" {
			stderr = "unknown error"
		}
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %s\n\nCheck that the identity file matches the recipient.", stderr),
		}
	}
	return out, nil
}
