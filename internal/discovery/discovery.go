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
// under files/ and the platform-scoped files.d/{tag}/ trees, then merges them
// with the explicit [[file]] entries from cfg.
//
// Precedence (ADR 004), lowest to highest:
//   - files/ entries (unscoped)
//   - files.d/{tag}/ entries for each tag in platforms; these carry Only={tag}
//     and override a same-destination files/ entry (later tag wins)
//   - explicit [[file]] entries whose src matches override everything
//
// Files ending in .age or .j2 are detected and flagged, but left to deploy to
// handle (or skip).
func Walk(cfg config.Config, platforms []string) ([]config.FileEntry, error) {
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

	for _, tag := range platforms {
		tagDir := filepath.Join(cfg.RepoRoot, "files.d", tag)
		info, err := os.Stat(tagDir)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := walkDir(tagDir, tagDir, cfg.RepoRoot, home, []string{tag})
		if err != nil {
			return nil, err
		}
		// files.d entries override a same-destination files/ entry (later wins).
		for _, e := range entries {
			discovered = overrideByDst(discovered, e)
		}
	}

	return mergeEntries(discovered, cfg.Files), nil
}

// overrideByDst replaces the first entry in list sharing entry's Dst, or
// appends entry if none matches.
func overrideByDst(list []config.FileEntry, entry config.FileEntry) []config.FileEntry {
	for i, e := range list {
		if e.Dst == entry.Dst {
			list[i] = entry
			return list
		}
	}
	return append(list, entry)
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
