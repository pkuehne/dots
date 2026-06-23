package config

import (
	"reflect"
	"sort"
	"testing"
)

// TestValidatorSpecsMatchStructs guards against the schema drifting away from
// the structs it is meant to mirror. If someone adds a field to one of the
// config structs but forgets to list its key here, the "unknown key" gap that
// produced issue #29 would silently reopen for that field — this test fails
// instead. Free-form sections (env vars, SSH options) have no backing struct of
// fixed fields and are intentionally omitted.
func TestValidatorSpecsMatchStructs(t *testing.T) {
	cases := []struct {
		name string
		spec map[string]valKind
		typ  reflect.Type
	}{
		{"meta", metaSpec, reflect.TypeOf(MetaConfig{})},
		{"shell", shellSpec, reflect.TypeOf(ShellConfig{})},
		{"git", gitSpec, reflect.TypeOf(GitConfig{})},
		{"ssh", sshSpec, reflect.TypeOf(SSHConfig{})},
		{"ssh.host", sshHostSpec, reflect.TypeOf(SSHHost{})},
		{"tools", toolsSpec, reflect.TypeOf(ToolsConfig{})},
		{"tool", toolSpec, reflect.TypeOf(Tool{})},
		{"tool.install", installSpec, reflect.TypeOf(ToolInstall{})},
		{"tool.shell", toolShellSpec, reflect.TypeOf(ToolShell{})},
		{"tool.git", toolGitSpec, reflect.TypeOf(ToolGit{})},
		{"file", fileSpec, reflect.TypeOf(FileEntry{})},
		{"repo", repoSpec, reflect.TypeOf(RepoEntry{})},
		{"secrets", secretsSpec, reflect.TypeOf(SecretsConfig{})},
		{"presets", presetsSpec, reflect.TypeOf(PresetsConfig{})},
		{"env.when", envWhenSpec, reflect.TypeOf(EnvWhen{})},
	}
	for _, c := range cases {
		if got, want := specKeys(c.spec), structTags(c.typ); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: spec keys %v do not match struct toml tags %v", c.name, got, want)
		}
	}
}

func specKeys(spec map[string]valKind) []string {
	keys := make([]string, 0, len(spec))
	for k := range spec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// structTags returns the toml tag names of a struct's fields, skipping fields
// tagged `-` (decoded manually, e.g. free-form SSH options).
func structTags(t reflect.Type) []string {
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("toml")
		if tag == "" || tag == "-" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
