// Package discovery walks the files/ directory and produces the list of
// FileEntry values that dots apply will deploy.
package discovery

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/fileutil"
)

// Walk returns all FileEntry values for the given repo. It discovers files
// under files/ and merges them with the explicit [[file]] entries from cfg:
// an explicit entry whose src matches a discovered path overrides it;
// otherwise it is appended.
//
// Files ending in .age or .j2 are detected and flagged, but left to deploy to
// handle (or skip). Platform-scoped files.d/{platform}/ discovery is not yet
// implemented.
func Walk(cfg config.Config, platform string) ([]config.FileEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var discovered []config.FileEntry

	filesDir := filepath.Join(cfg.RepoRoot, "files")
	if info, err := os.Stat(filesDir); err == nil && info.IsDir() {
		entries, err := walkDir(filesDir, filesDir, cfg.RepoRoot, home, nil)
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, entries...)
	}

	return mergeEntries(discovered, cfg.Files), nil
}

// walkDir recurses into dir, producing a FileEntry for each file.
// baseDir is the discovery root (files/ or files.d/{plat}/) used to compute
// the destination path relative to home.
func walkDir(dir, baseDir, repoRoot, home string, only []string) ([]config.FileEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []config.FileEntry
	for _, de := range entries {
		name := de.Name()
		if fileutil.ShouldSkip(name) {
			continue
		}
		full := filepath.Join(dir, name)
		if de.IsDir() {
			sub, err := walkDir(full, baseDir, repoRoot, home, only)
			if err != nil {
				return nil, err
			}
			result = append(result, sub...)
			continue
		}

		rel, err := filepath.Rel(baseDir, full)
		if err != nil {
			return nil, err
		}
		srcRel, err := filepath.Rel(repoRoot, full)
		if err != nil {
			return nil, err
		}

		dst := filepath.Join(home, rel)
		entry := config.FileEntry{
			Src:  srcRel,
			Dst:  dst,
			Only: only,
		}
		switch {
		case strings.HasSuffix(name, ".age"):
			entry.Secret = true
			entry.Dst = strings.TrimSuffix(dst, ".age")
		case strings.HasSuffix(name, ".j2"):
			entry.Template = true
			entry.Dst = strings.TrimSuffix(dst, ".j2")
		}
		result = append(result, entry)
	}
	return result, nil
}

// mergeEntries merges explicit [[file]] entries into the discovered list.
// An explicit entry whose src matches a discovered entry replaces it;
// otherwise it is appended.
func mergeEntries(discovered, explicit []config.FileEntry) []config.FileEntry {
	result := make([]config.FileEntry, len(discovered))
	copy(result, discovered)
	for _, exp := range explicit {
		replaced := false
		for i, disc := range result {
			if disc.Src == exp.Src {
				result[i] = exp
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, exp)
		}
	}
	return result
}
