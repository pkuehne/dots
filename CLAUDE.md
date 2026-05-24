# dots — developer guide

## Architecture

See docs/architecture.md for a full overview.

This branch (`feat/migrate-to-golang`) is a Go rewrite. The Python source under
`src/dots/` is kept for reference. The Go code lives under `cmd/` and `internal/`.

## Commands

```sh
# Build
just build          # → bin/dots
go build ./...      # compile check

# Test
just test           # go test ./...

# Format / vet
just fmt
just vet

# Python tests (kept for reference during migration)
pytest tests/
```

## Commits

This project uses [Conventional Commits](https://www.conventionalcommits.org/) and release-please for automated releases.

```
feat: add SSH managed mode        # bumps minor
fix: handle stale symlinks        # bumps patch
feat!: breaking change            # bumps major (minor while < 1.0)
chore: update CI                  # no version bump
docs: update README               # no version bump
test: add e2e scenario            # no version bump
refactor: split cli module        # no version bump
```

Always use a conventional prefix. Keep the subject line under 70 characters.
Use the body for detail when needed.

## Go package layout

```
cmd/dots/         CLI entry point + cobra command stubs
internal/config/  Config structs + Load() (stub)
internal/platform/ OS/arch detection (implemented)
internal/errs/    DotsError, ConfigError, ToolInstallError
internal/discovery/ File discovery (stub)
internal/deploy/  File deployment — symlink/copy/render (stub)
internal/shell/   Shell snippet generation (stub)
internal/git/     Git config generation (stub)
internal/ssh/     SSH config generation (stub)
internal/tools/   Tool installation (stub)
internal/repos/   Repo cloning (stub)
internal/secrets/ age encrypt/decrypt (stub)
internal/presets/ Preset generation (stub)
```

## Key invariants

1. dots binary lives at cmd/dots/main.go. Entry: cobra root command.
2. No mandatory third-party imports beyond cobra and BurntSushi/toml.
3. Every user-facing operation is idempotent. Running twice = same result.
4. No operation modifies anything outside ~. No /etc, no /usr.
5. Dry run (--dry-run) must produce zero side effects.
6. Every error has a Hint. Never show a raw traceback.
7. Generated files always have a header saying they are generated.
8. No user-editable file is ever silently overwritten without a backup.

## Adding a new managed subsystem

1. Add a dataclass in the Config section (e.g. FooConfig)
2. Add `foo: FooConfig` to the Config dataclass
3. Parse it in load_config()
4. Add generate_foo() function
5. Add cmd_foo_init(), cmd_foo_show(), cmd_foo_uninit()
6. Wire into cmd_apply() execution order
7. Wire into build_parser()
8. Wire into main() dispatch
9. Add doctor checks
10. Add unit tests in tests/unit/test_foo_config.py
11. Add integration tests in tests/integration/test_foo_managed.py
12. Write adr/NNN-foo-managed.md

## Adding a new install method

1. Add method name to ToolInstall.method type annotation
2. Add a branch in install_tool() dispatch
3. Add to the install method reference table in the spec and docs
4. Add test cases in tests/unit/test_tools.py
5. Document in docs/configuration.md

## Common patterns (to be implemented)

- `expand(path)`: resolve `~` and `$VAR` → absolute path
- `run(cmd)`: exec with structured error wrapping
- `idempotentInsert(path, content, marker)`: marker-delimited block insert/update
- `sha256File(path)`: content hash for change detection
- `backup(path)`: copy to `path.dots-bak` before overwriting
