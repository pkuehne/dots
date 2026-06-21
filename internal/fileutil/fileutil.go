// Package fileutil provides path expansion, file hashing, and safe filesystem
// operations used across the dots subsystems.
package fileutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// skipNames is the set of file/dir names always ignored during discovery.
var skipNames = map[string]bool{
	".git":      true,
	".DS_Store": true,
}

// skipSuffixes is the set of file suffixes always ignored during discovery.
var skipSuffixes = []string{".swp"}

// sensitiveDirModes maps directory names that must be created with restricted
// permissions (e.g. ~/.ssh must be 0700).
var sensitiveDirModes = map[string]os.FileMode{
	".ssh":   0o700,
	".gnupg": 0o700,
}

// Expand resolves ~ and $VAR references to an absolute path.
func Expand(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// ShouldSkip reports whether a file or directory name should be ignored during
// discovery (version control artifacts, editor temporaries, etc.).
func ShouldSkip(name string) bool {
	if skipNames[name] {
		return true
	}
	for _, s := range skipSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return strings.HasSuffix(name, "~")
}

// EnsureParent creates all parent directories of path. Directories named in
// sensitiveDirModes are created with their restricted permissions; all others
// use 0755.
func EnsureParent(path string) error {
	return ensureDir(filepath.Dir(path))
}

// ensureDir creates dir and all its ancestors, applying sensitive-dir
// permissions where applicable.
func ensureDir(dir string) error {
	if info, err := os.Lstat(dir); err == nil {
		// Resolve symlinks: a symlink pointing at a directory is a valid
		// parent. Only a non-directory (or a dangling/non-dir symlink) is an
		// error. Using Lstat's result directly would reject every
		// symlinked directory as if it were a regular file.
		if info.Mode()&os.ModeSymlink != 0 {
			if target, terr := os.Stat(dir); terr != nil || !target.IsDir() {
				return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrExist}
			}
			return nil
		}
		if !info.IsDir() {
			return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrExist}
		}
		if mode, ok := sensitiveDirModes[filepath.Base(dir)]; ok {
			_ = os.Chmod(dir, mode)
		}
		return nil
	}

	// Create parent first, then this dir.
	if parent := filepath.Dir(dir); parent != dir {
		if err := ensureDir(parent); err != nil {
			return err
		}
	}

	mode := os.FileMode(0o755)
	if m, ok := sensitiveDirModes[filepath.Base(dir)]; ok {
		mode = m
	}
	return os.Mkdir(dir, mode)
}

// Backup copies path to path.dots-bak (preserving symlinks as symlinks).
// Returns the backup path, or "" if path did not exist.
func Backup(path string) (string, error) {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return "", nil
	}

	bak := path + ".dots-bak"
	_ = os.Remove(bak)

	// Preserve symlinks: re-create the symlink rather than copying the target.
	if target, err := os.Readlink(path); err == nil {
		if err := os.Symlink(target, bak); err != nil {
			return "", err
		}
		return bak, nil
	}

	if err := copyFile(path, bak); err != nil {
		return "", err
	}
	return bak, nil
}

// WriteIfChanged writes content to path only when the current content
// differs (creating parent directories as needed), keeping generated-file
// writes idempotent. Returns true when the file was — or with dryRun would
// be — written.
func WriteIfChanged(path string, content []byte, mode os.FileMode, dryRun bool) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if err := EnsureParent(path); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, content, mode)
}

// SHA256File returns the hex SHA-256 digest of a file's contents.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyFile copies src to dst, preserving permissions.
func CopyFile(src, dst string) error { return copyFile(src, dst) }

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
