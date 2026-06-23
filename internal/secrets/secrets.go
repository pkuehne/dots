// Package secrets wraps age for encrypting and decrypting .age files.
package secrets

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
)

// Encrypt encrypts src using cfg.Secrets.Recipient and writes to dst.
// If dst is empty, dst = src + ".age".
func Encrypt(src, dst string, cfg config.Config) error {
	if cfg.Secrets.Recipient == "" {
		return &errs.DotsError{
			Msg:  "No recipient configured for encryption",
			Hint: "Set in dots.toml:\n  [secrets]\n  recipient = \"age1...\"",
		}
	}
	recipients, err := age.ParseRecipients(strings.NewReader(cfg.Secrets.Recipient))
	if err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: invalid recipient %q: %v", cfg.Secrets.Recipient, err),
		}
	}

	plaintext, err := os.ReadFile(src)
	if err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}

	out := dst
	if out == "" {
		out = src + ".age"
	}
	if err := fileutil.EnsureParent(out); err != nil {
		return err
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}
	if _, err := w.Write(plaintext); err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}
	if err := w.Close(); err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to encrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}
	return writeFile(out, buf.Bytes())
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
	identityPath := fileutil.Expand(cfg.Secrets.Identity)
	if !fileExists(identityPath) {
		return nil, &errs.DotsError{
			Msg: fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: age identity file not found: %s\n\n"+
				"Hint: Generate an age keypair with:\n"+
				"  dots secrets keygen\n"+
				"Then set the public key as recipient in dots.toml:\n"+
				"  [secrets]\n"+
				"  recipient = \"age1...\"",
				identityPath),
		}
	}

	keyFile, err := os.Open(identityPath)
	if err != nil {
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: cannot read identity file: %v", err),
		}
	}
	defer keyFile.Close()

	identities, err := age.ParseIdentities(keyFile)
	if err != nil {
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: invalid identity file: %v", err),
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}
	defer in.Close()

	r, err := age.Decrypt(in, identities...)
	if err != nil {
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v\n\nCheck that the identity file matches the recipient.", err),
		}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, &errs.DotsError{
			Msg:  fmt.Sprintf("Failed to decrypt %s", baseName(src)),
			Hint: fmt.Sprintf("Reason: %v", err),
		}
	}
	return data, nil
}
