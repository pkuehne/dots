// Package discovery walks the files/ and files.d/{platform}/ directories and
// produces the list of FileEntry values that dots apply will deploy.
package discovery

import "github.com/pkuehne/dots/internal/config"

// Walk returns all discovered FileEntry values for the given repo root and
// platform. Explicit [[file]] entries from cfg are merged in: an entry whose
// src matches a discovered path overrides it; otherwise it is appended.
func Walk(cfg config.Config, platform string) ([]config.FileEntry, error) {
	panic("Walk: not yet implemented")
}
