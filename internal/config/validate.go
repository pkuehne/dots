package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkuehne/dots/internal/errs"
)

// This file is the strict-validation pass that runs before the lenient,
// map-based parsers in parse.go. The parsers deliberately fall back to defaults
// on anything they don't recognise (so that e.g. SSH options can be free-form),
// which means a misspelled key or a structurally wrong value would otherwise be
// dropped silently and resurface later as a confusing downstream error — the
// class of bug behind issue #29. validateConfig closes that gap: it rejects
// unknown keys (with a "did you mean" suggestion) and values whose shape does
// not match the schema, with an actionable hint, before parsing begins.

// valKind is the structural shape a config value is expected to have. We check
// shape (scalar vs. array vs. table vs. array-of-tables), not scalar subtype:
// SSH options and a few other fields are intentionally written as native
// scalars (port = 2222), so int-vs-string coercion stays the parser's job.
type valKind int

const (
	kScalar     valKind = iota // string, bool, int, float, datetime
	kArray                     // array of scalars: ["a", "b"]
	kTable                     // a [section] or inline { … } table
	kTableArray                // [[array.of.tables]] or x = [ { … } ]
)

func (k valKind) String() string {
	switch k {
	case kScalar:
		return "a single value"
	case kArray:
		return "an array"
	case kTable:
		return "a table"
	case kTableArray:
		return "an array of tables"
	}
	return "?"
}

// ── Schemas ──────────────────────────────────────────────────────────────────
//
// One spec per section. These are kept honest against the structs in config.go
// by TestValidatorSpecsMatchStructs, which fails if a struct field gains a toml
// tag that is not listed here — so adding a field can't silently reintroduce
// the "unknown key" gap.

var (
	topSpec = map[string]valKind{
		"meta": kTable, "shell": kTable, "git": kTable, "ssh": kTable,
		"tools": kTable, "secrets": kTable, "presets": kTable, "env": kTable,
		"profiles": kTable,
		"tool":     kTableArray, "file": kTableArray, "repo": kTableArray,
	}
	metaSpec      = map[string]valKind{"version": kScalar, "default_mode": kScalar}
	shellSpec     = map[string]valKind{"managed": kScalar, "login": kScalar, "zshrc": kScalar, "bashrc": kScalar, "dir": kScalar, "path": kArray}
	gitSpec       = map[string]valKind{"managed": kScalar, "name": kScalar, "email": kScalar, "editor": kScalar, "default_branch": kScalar, "pull_rebase": kScalar, "signingkey": kScalar, "sign": kScalar}
	sshSpec       = map[string]valKind{"managed": kScalar, "host": kTableArray}
	sshHostSpec   = map[string]valKind{"host": kScalar, "only": kArray} // extra keys allowed: free-form SSH options
	toolsSpec     = map[string]valKind{"bin_dir": kScalar}
	toolSpec      = map[string]valKind{"name": kScalar, "desc": kScalar, "check": kScalar, "tags": kArray, "only": kArray, "profile": kScalar, "install": kTableArray, "shell": kTable, "git": kTable}
	installSpec   = map[string]valKind{"method": kScalar, "package": kScalar, "repo": kScalar, "asset": kScalar, "binary": kScalar, "binary_path": kScalar, "install_dir": kScalar, "strip_components": kScalar, "version": kScalar, "script": kScalar, "note": kScalar, "only": kArray, "arch_map": kTable}
	toolShellSpec = map[string]valKind{"env": kTable, "init": kScalar, "path": kArray}
	toolGitSpec   = map[string]valKind{"pager": kScalar, "diff": kScalar}
	fileSpec      = map[string]valKind{"src": kScalar, "dst": kScalar, "only": kArray, "profile": kScalar, "secret": kScalar, "mode": kScalar, "link": kScalar}
	repoSpec      = map[string]valKind{"name": kScalar, "repo": kScalar, "dst": kScalar, "shallow": kScalar, "ref": kScalar, "on_install": kScalar, "on_update": kScalar, "only": kArray, "profile": kScalar}
	secretsSpec   = map[string]valKind{"recipient": kScalar, "identity": kScalar}
	presetsSpec   = map[string]valKind{"fzf": kScalar, "tmux": kScalar}
	envWhenSpec   = map[string]valKind{"key": kScalar, "value": kScalar, "if_tool": kScalar, "only": kArray}
)

// ── Entry point ──────────────────────────────────────────────────────────────

// validateConfig checks a decoded (and profile-merged) TOML document against
// the schema and returns the first violation as a ConfigError. It is a
// best-effort guard against typos and shape errors, not a full type-checker.
func validateConfig(raw map[string]any) error {
	if err := checkKeys(raw, topSpec, "top level", false); err != nil {
		return err
	}
	if m, ok := raw["meta"].(map[string]any); ok {
		if err := checkKeys(m, metaSpec, "[meta]", false); err != nil {
			return err
		}
	}
	if m, ok := raw["shell"].(map[string]any); ok {
		if err := checkKeys(m, shellSpec, "[shell]", false); err != nil {
			return err
		}
	}
	if m, ok := raw["git"].(map[string]any); ok {
		if err := checkKeys(m, gitSpec, "[git]", false); err != nil {
			return err
		}
	}
	if m, ok := raw["ssh"].(map[string]any); ok {
		if err := checkKeys(m, sshSpec, "[ssh]", false); err != nil {
			return err
		}
		for _, h := range tableSlice(m, "host") {
			// Free-form SSH options are expected, so allow extra keys; we still
			// type-check the structural ones (host, only).
			if err := checkKeys(h, sshHostSpec, "[[ssh.host]]", true); err != nil {
				return err
			}
		}
	}
	if m, ok := raw["tools"].(map[string]any); ok {
		if err := checkKeys(m, toolsSpec, "[tools]", false); err != nil {
			return err
		}
	}
	for _, t := range tableSlice(raw, "tool") {
		if err := checkKeys(t, toolSpec, "[[tool]]", false); err != nil {
			return err
		}
		for _, ins := range tableSlice(t, "install") {
			if err := checkKeys(ins, installSpec, "[[tool.install]]", false); err != nil {
				return err
			}
		}
		if sh, ok := t["shell"].(map[string]any); ok {
			if err := checkKeys(sh, toolShellSpec, "[tool.shell]", false); err != nil {
				return err
			}
		}
		if g, ok := t["git"].(map[string]any); ok {
			if err := checkKeys(g, toolGitSpec, "[tool.git]", false); err != nil {
				return err
			}
		}
	}
	for _, f := range tableSlice(raw, "file") {
		if err := checkKeys(f, fileSpec, "[[file]]", false); err != nil {
			return err
		}
	}
	for _, r := range tableSlice(raw, "repo") {
		if err := checkKeys(r, repoSpec, "[[repo]]", false); err != nil {
			return err
		}
	}
	if m, ok := raw["secrets"].(map[string]any); ok {
		if err := checkKeys(m, secretsSpec, "[secrets]", false); err != nil {
			return err
		}
	}
	if m, ok := raw["presets"].(map[string]any); ok {
		if err := checkKeys(m, presetsSpec, "[presets]", false); err != nil {
			return err
		}
	}
	if err := validateEnv(raw); err != nil {
		return err
	}
	// Each profile is a partial config subtree; validate it the same way so a
	// typo inside an inactive profile is caught too, not just the active one.
	if profiles, ok := raw["profiles"].(map[string]any); ok {
		for name, p := range profiles {
			sub, ok := p.(map[string]any)
			if !ok {
				return errs.NewConfig(
					fmt.Sprintf("[profiles.%s] must be a table", name),
					"A profile holds config overrides, e.g.\n  [profiles.linux]\n  [[profiles.linux.tool]]\n  name = \"ripgrep\"",
				)
			}
			if err := validateConfig(sub); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateEnv handles the [env] section, which uniquely mixes free-form
// UPPERCASE scalar vars with [[env.when]] tables. Var names and values are left
// to parseEnv; here we only police the reserved "when" key's shape and the
// fields of each when-entry.
func validateEnv(raw map[string]any) error {
	env, ok := raw["env"].(map[string]any)
	if !ok {
		return nil
	}
	if w, present := env["when"]; present {
		if !kindMatches(kTableArray, w) {
			return typeErr("when", kTableArray, w, "[env]")
		}
	}
	for _, e := range tableSlice(env, "when") {
		if err := checkKeys(e, envWhenSpec, "[[env.when]]", false); err != nil {
			return err
		}
	}
	return nil
}

// ── Checking primitives ──────────────────────────────────────────────────────

// checkKeys validates every key in section against spec. With allowExtra, keys
// not in spec are permitted (used for sections with free-form keys) but any key
// that *is* in spec is still shape-checked.
func checkKeys(section map[string]any, spec map[string]valKind, ctx string, allowExtra bool) error {
	for key, v := range section {
		want, known := spec[key]
		if !known {
			if allowExtra {
				continue
			}
			return unknownKeyErr(key, spec, ctx)
		}
		if !kindMatches(want, v) {
			return typeErr(key, want, v, ctx)
		}
	}
	return nil
}

func kindMatches(want valKind, v any) bool {
	switch want {
	case kScalar:
		return isScalar(v)
	case kTable:
		_, ok := v.(map[string]any)
		return ok
	case kArray:
		arr, ok := v.([]any)
		if !ok {
			return false
		}
		for _, e := range arr {
			if !isScalar(e) {
				return false
			}
		}
		return true
	case kTableArray:
		switch a := v.(type) {
		case []map[string]any:
			return true
		case []any:
			for _, e := range a {
				if _, ok := e.(map[string]any); !ok {
					return false
				}
			}
			return true
		}
		return false
	}
	return false
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any, []map[string]any:
		return false
	default:
		return true
	}
}

// actualKind describes the shape a value actually has, for error messages.
func actualKind(v any) valKind {
	switch a := v.(type) {
	case map[string]any:
		return kTable
	case []map[string]any:
		return kTableArray
	case []any:
		for _, e := range a {
			if _, ok := e.(map[string]any); ok {
				return kTableArray
			}
		}
		return kArray
	default:
		return kScalar
	}
}

// ── Error construction ───────────────────────────────────────────────────────

func unknownKeyErr(key string, spec map[string]valKind, ctx string) error {
	hint := "Valid keys in " + ctx + ": " + strings.Join(sortedKeys(spec), ", ") + "."
	if s := closestKey(key, spec); s != "" {
		hint = fmt.Sprintf("Did you mean %q? ", s) + hint
	}
	return errs.NewConfig(fmt.Sprintf("unknown key %q in %s", key, ctx), hint)
}

func typeErr(key string, want valKind, v any, ctx string) error {
	hint := fmt.Sprintf("%q must be %s.", key, want)
	if want == kTableArray {
		hint += fmt.Sprintf(" Use either form:\n  [[%s]]\n  …\nor\n  %s = [ { … } ]", key, key)
	}
	return errs.NewConfig(
		fmt.Sprintf("%s key %q must be %s, but got %s", ctx, key, want, actualKind(v)),
		hint,
	)
}

func sortedKeys(spec map[string]valKind) []string {
	keys := make([]string, 0, len(spec))
	for k := range spec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// closestKey returns the spec key nearest to key by edit distance, if a close
// match exists (threshold scales with key length so short keys aren't matched
// to anything). Returns "" when nothing is close enough to be a likely typo.
func closestKey(key string, spec map[string]valKind) string {
	best := ""
	bestDist := 1 << 30
	for k := range spec {
		d := levenshtein(key, k)
		if d < bestDist {
			bestDist, best = d, k
		}
	}
	limit := len(key)/2 + 1
	if bestDist <= limit {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
