package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkuehne/dots/internal/errs"
)

// ── Utilities ────────────────────────────────────────────────────────────────

func str(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func boolean(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func intVal(m map[string]any, key string, def int64) int64 {
	if v, ok := m[key].(int64); ok {
		return v
	}
	return def
}

// scalarToString renders a TOML scalar (string, int, float, bool) as a string.
// It reports false for non-scalar values (tables, arrays) so callers can skip
// them rather than store a Go-syntax representation. Used for SSH options,
// where values are free-form keywords that may be written as native scalars.
func scalarToString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		if x {
			return "yes", true
		}
		return "no", true
	case int64:
		return fmt.Sprintf("%d", x), true
	case float64:
		return fmt.Sprintf("%g", x), true
	default:
		return "", false
	}
}

func strSlice(m map[string]any, key string) []string {
	v, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func strMap(m map[string]any, key string) map[string]string {
	v, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(v))
	for k, val := range v {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

// tableSlice extracts an array of tables from m[key].
// BurntSushi/toml decodes [[array-of-tables]] as []map[string]interface{},
// which is the same underlying type as []map[string]any.
func tableSlice(m map[string]any, key string) []map[string]any {
	v, ok := m[key].([]map[string]any)
	if !ok {
		return nil
	}
	return v
}

// ── Document loading ─────────────────────────────────────────────────────────

// parseDoc reads and decodes a TOML file into a raw map.
func parseDoc(path string) (map[string]any, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, errs.NewConfig(
			"failed to parse "+filepath.Base(path),
			err.Error(),
		)
	}
	return raw, nil
}

// deepMerge returns a new map with override values merged recursively into base.
// Sub-tables are merged recursively; all other types are replaced.
func deepMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		if bv, ok := result[k]; ok {
			if bmap, ok := bv.(map[string]any); ok {
				if omap, ok := v.(map[string]any); ok {
					result[k] = deepMerge(bmap, omap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

// mergeProfiles applies platform, hostname, and manual profile overlays in that
// order. Later profiles win.
func mergeProfiles(raw map[string]any, plat, hostname, profile string) map[string]any {
	profiles, _ := raw["profiles"].(map[string]any)
	result := raw
	for _, key := range []string{plat, hostname, profile} {
		if key == "" {
			continue
		}
		if override, ok := profiles[key].(map[string]any); ok {
			result = deepMerge(result, override)
		}
	}
	return result
}

// ── Section parsers ──────────────────────────────────────────────────────────

func parseEnv(raw map[string]any) (EnvConfig, error) {
	cfg := EnvConfig{Vars: make(map[string]string)}
	section, ok := raw["env"].(map[string]any)
	if !ok {
		return cfg, nil
	}

	for key, val := range section {
		if key == "when" {
			continue
		}
		s, ok := val.(string)
		if !ok {
			s = fmt.Sprintf("%v", val)
		}
		if key == "PATH" {
			return cfg, errs.NewConfig(
				"PATH must not appear in [env]",
				"Use [shell] path instead:\n"+
					"  [shell]\n  path = [\"~/.local/bin\", \"~/.cargo/bin\"]",
			)
		}
		if strings.ToUpper(key) != key {
			return cfg, errs.NewConfig(
				fmt.Sprintf("environment key %q must be UPPERCASE", key),
				fmt.Sprintf("Rename to: %s = %q", strings.ToUpper(key), s),
			)
		}
		if _, dup := cfg.Vars[key]; dup {
			return cfg, errs.NewConfig(
				"duplicate environment key: "+key,
				"Remove the duplicate entry from [env].",
			)
		}
		cfg.Vars[key] = s
	}

	for _, item := range tableSlice(section, "when") {
		ew := EnvWhen{
			Key:    str(item, "key", ""),
			Value:  str(item, "value", ""),
			IfTool: str(item, "if_tool", ""),
			Only:   strSlice(item, "only"),
		}
		if ew.Key == "" || ew.Value == "" {
			return cfg, errs.NewConfig(
				"[[env.when]] entry missing required 'key' or 'value'",
				"Each [[env.when]] needs at minimum:\n"+
					"  key = \"VAR_NAME\"\n  value = \"the value\"",
			)
		}
		cfg.When = append(cfg.When, ew)
	}

	return cfg, nil
}

func parseSSHHosts(raw map[string]any) []SSHHost {
	var hosts []SSHHost
	for _, item := range tableSlice(raw, "host") {
		h := SSHHost{
			Host:    str(item, "host", ""),
			Only:    strSlice(item, "only"),
			Options: make(map[string]string),
		}
		for k, v := range item {
			if k == "host" || k == "only" {
				continue
			}
			// SSH options are written as native TOML scalars — a port is most
			// naturally `port = 2222`, not `port = "2222"`. Coerce ints, floats,
			// and bools to their string form so they are not silently dropped.
			if s, ok := scalarToString(v); ok {
				h.Options[k] = s
			}
		}
		hosts = append(hosts, h)
	}
	return hosts
}

func parseToolInstall(raw map[string]any) ToolInstall {
	return ToolInstall{
		Method:     str(raw, "method", ""),
		Package:    str(raw, "package", ""),
		Repo:       str(raw, "repo", ""),
		Asset:      str(raw, "asset", ""),
		Binary:     str(raw, "binary", ""),
		BinaryPath: str(raw, "binary_path", ""),
		Version:    str(raw, "version", ""),
		Script:     str(raw, "script", ""),
		Note:       str(raw, "note", ""),
		Only:       strSlice(raw, "only"),
		ArchMap:    strMap(raw, "arch_map"),
	}
}

func parseTool(raw map[string]any) (Tool, error) {
	t := Tool{
		Name:    str(raw, "name", ""),
		Desc:    str(raw, "desc", ""),
		Check:   str(raw, "check", ""),
		Tags:    strSlice(raw, "tags"),
		Only:    strSlice(raw, "only"),
		Profile: str(raw, "profile", ""),
	}
	if t.Name == "" {
		return Tool{}, errs.NewConfig(
			"[[tool]] entry missing required 'name' field",
			"Every tool needs a name:\n  [[tool]]\n  name = \"ripgrep\"",
		)
	}
	if t.Check == "" {
		t.Check = "which " + t.Name
	}
	for _, ri := range tableSlice(raw, "install") {
		t.Install = append(t.Install, parseToolInstall(ri))
	}
	if rs, ok := raw["shell"].(map[string]any); ok {
		t.Shell = ToolShell{
			Env:  strMap(rs, "env"),
			Init: str(rs, "init", ""),
			Path: strSlice(rs, "path"),
		}
	}
	if rg, ok := raw["git"].(map[string]any); ok {
		t.Git = ToolGit{
			Pager: boolean(rg, "pager", false),
			Diff:  boolean(rg, "diff", false),
		}
	}
	return t, nil
}

func parseFileEntry(raw map[string]any) (FileEntry, error) {
	src := str(raw, "src", "")
	dst := str(raw, "dst", "")
	if src == "" {
		return FileEntry{}, errs.NewConfig(
			"[[file]] entry missing required 'src' field",
			"Every file entry needs:\n  [[file]]\n"+
				"  src = \"files/.gitconfig\"\n  dst = \"~/.gitconfig\"",
		)
	}
	if dst == "" {
		return FileEntry{}, errs.NewConfig(
			fmt.Sprintf("[[file]] entry for %q missing required 'dst' field", src),
			"Add a destination:\n  dst = \"~/.gitconfig\"",
		)
	}
	fe := FileEntry{
		Src:      src,
		Dst:      dst,
		Only:     strSlice(raw, "only"),
		Profile:  str(raw, "profile", ""),
		Template: boolean(raw, "template", false),
		Secret:   boolean(raw, "secret", false),
		Mode:     str(raw, "mode", ""),
	}
	if v, ok := raw["link"].(bool); ok {
		fe.Link = &v
	}
	return fe, nil
}

func parseRepoEntry(raw map[string]any) (RepoEntry, error) {
	name := str(raw, "name", "")
	if name == "" {
		return RepoEntry{}, errs.NewConfig(
			"[[repo]] entry missing required 'name' field",
			"Every repo needs a name:\n  [[repo]]\n  name = \"tpm\"",
		)
	}
	return RepoEntry{
		Name:      name,
		Repo:      str(raw, "repo", ""),
		Dst:       str(raw, "dst", ""),
		Shallow:   boolean(raw, "shallow", false),
		Ref:       str(raw, "ref", ""),
		OnInstall: str(raw, "on_install", ""),
		OnUpdate:  str(raw, "on_update", ""),
		Only:      strSlice(raw, "only"),
		Profile:   str(raw, "profile", ""),
	}, nil
}
