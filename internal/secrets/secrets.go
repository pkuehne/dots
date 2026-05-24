// Package secrets wraps age for encrypting and decrypting .age files.
package secrets

import "github.com/pkuehne/dots/internal/config"

// Encrypt encrypts src using cfg.Secrets.Recipient and writes to dst.
// If dst is empty, dst = src + ".age".
func Encrypt(src, dst string, cfg config.Config) error {
	panic("Encrypt: not yet implemented")
}

// Decrypt decrypts src using cfg.Secrets.Identity and writes to dst.
// If dst is empty, dst is src with the ".age" suffix removed.
func Decrypt(src, dst string, cfg config.Config) error {
	panic("Decrypt: not yet implemented")
}

// DecryptToMemory returns the plaintext of an .age file without writing to disk.
// Used during apply to render decrypted files in-memory.
func DecryptToMemory(src string, cfg config.Config) ([]byte, error) {
	panic("DecryptToMemory: not yet implemented")
}
