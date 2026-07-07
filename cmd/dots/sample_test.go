package main

import (
	"os"
	"path/filepath"
	"reflect"
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

// TestSampleConfigIsComprehensive walks the config structs and asserts every
// toml-tagged key is at least mentioned in the sample (as a set value or in a
// comment). TestSampleConfigIsValid guarantees the sample contains no key that
// isn't in the schema; this test guards the other direction — the schema
// contains no key the sample fails to document.
func TestSampleConfigIsComprehensive(t *testing.T) {
	keys := map[string]bool{}
	collectTomlTags(reflect.TypeOf(config.Config{}), keys, map[reflect.Type]bool{})

	for key := range keys {
		if !strings.Contains(sampleConfig, key) {
			t.Errorf("schema key %q is not documented in the sample config", key)
		}
	}
}

// collectTomlTags recursively gathers toml tag names from t and its field
// types (descending through pointers, slices, and maps).
func collectTomlTags(t reflect.Type, keys map[string]bool, seen map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag := f.Tag.Get("toml"); tag != "" && tag != "-" {
			keys[strings.Split(tag, ",")[0]] = true
		}
		collectTomlTags(f.Type, keys, seen)
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
