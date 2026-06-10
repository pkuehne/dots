package secrets

import (
	"os"
	"path/filepath"
)

func baseName(path string) string { return filepath.Base(path) }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
