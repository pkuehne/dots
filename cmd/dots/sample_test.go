package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
)

// TestSampleConfigIsValid loads the annotated sample through the real parser and
// validator. Because validateConfig rejects unknown keys and wrong shapes, this
// guarantees every key documented in sample.go is a genuine, current schema key
// — if the schema changes and sample.go isn't updated, this test fails.
func TestSampleConfigIsValid(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "dots.toml"), []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(repo, "")
	if err != nil {
		t.Fatalf("sample dots.toml failed to load: %v", err)
	}

	// Spot-check that representative sections actually parsed, so a sample that
	// silently degraded to defaults (e.g. a whole section mistyped) is caught.
	if !cfg.Shell.Managed {
		t.Error("expected [shell] managed = true to parse")
	}
	if cfg.Git.Email == "" {
		t.Error("expected [git] email to parse")
	}
	if len(cfg.Tools) == 0 {
		t.Error("expected [[tool]] entries to parse")
	}
	if len(cfg.Files) == 0 {
		t.Error("expected [[file]] entries to parse")
	}
	if len(cfg.Repos) == 0 {
		t.Error("expected [[repo]] entries to parse")
	}
	if _, ok := cfg.Profiles["work"]; !ok {
		t.Error("expected [profiles.work] to parse")
	}
}

// TestSampleConfigWorkProfileMerges confirms the documented profile-layering
// behaviour actually works when the sample's "work" profile is activated.
func TestSampleConfigWorkProfileMerges(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "dots.toml"), []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(repo, "work")
	if err != nil {
		t.Fatalf("sample dots.toml failed to load with work profile: %v", err)
	}
	if !strings.Contains(cfg.Git.Email, "work") {
		t.Errorf("work profile git email override not applied: got %q", cfg.Git.Email)
	}
}
