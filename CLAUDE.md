# dots — developer guide

## Architecture

See docs/architecture.md for a full overview.

The Go implementation lives under `cmd/` and `internal/`. The Python source has been removed (see ADR 014).

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
cmd/dots/          CLI entry point (main.go + commands.go) — implemented
internal/config/   Config structs + Load(), FindRepoRoot() — implemented
internal/platform/ OS/arch detection — implemented
internal/errs/     DotsError, ConfigError, ToolInstallError — implemented
internal/fileutil/ Expand, Sha256File, Backup, EnsureParent, CopyFile — implemented
internal/discovery/ File discovery (Walk) — implemented
internal/deploy/   File deployment — symlink/copy/decrypt (.age); .j2 templates unsupported — implemented
internal/shell/    Snippet generation + InsertBlock/RemoveBlock — implemented
internal/git/      Git config generation + WriteManaged/Uninit — implemented
internal/ssh/      SSH config generation + WriteManaged/Uninit — implemented
internal/secrets/  age encrypt/decrypt — implemented
internal/presets/  Preset generation + Eject — implemented
internal/repos/    Repo cloning + Update/Status — implemented
internal/tools/    Check, Install, Filter + GitHub release download — implemented
```

## Key invariants

1. dots binary lives at cmd/dots/main.go. Entry: cobra root command.
2. No mandatory third-party imports beyond cobra and BurntSushi/toml.
3. Every user-facing operation is idempotent. Running twice = same result.
4. File deployment never writes outside ~. No /etc, no /usr. (Tool install
   methods — apt/brew/pkg — install system packages by design and may use
   sudo; that is the only sanctioned path that touches system locations.)
5. Dry run (--dry-run) must produce zero side effects.
6. Every error has a Hint. Never show a raw traceback.
7. Generated files always have a header saying they are generated.
8. No user-editable file is ever silently overwritten without a backup.

## Adding a new managed subsystem

1. Add a struct (e.g. `FooConfig`) in `internal/config/config.go`
2. Add `Foo FooConfig` to the root `Config` struct
3. Parse it in `internal/config/load.go`
4. Create `internal/foo/foo.go` with `GenerateConfig`, `WriteManaged`, `Uninit`
5. Add cobra sub-commands `foo init`, `foo show`, `foo uninit` in `cmd/dots/commands.go`
6. Wire into `apply` orchestration in `cmd/dots/commands.go` (`newApplyCmd`)
7. Add doctor checks in `runDoctor()` in `cmd/dots/commands.go`
8. Write `internal/foo/foo_test.go` with table-driven tests using `t.TempDir()`
9. Write `adr/NNN-foo-managed.md`

## Adding a new install method

1. Add the method name to `ToolInstall.Method` in `internal/config/config.go`
2. Add a branch in `Install()` dispatch in `internal/tools/tools.go`
3. Add to the install method reference table in `docs/configuration.md`
4. Add test cases in `internal/tools/tools_test.go`
5. Document in `docs/configuration.md`

## Common helpers (use these, do not reimplement)

- `fileutil.Expand(path)`: resolve `~` and `$VAR` → absolute path
- `fileutil.Sha256File(path)`: content hash for change detection
- `fileutil.Backup(path)`: copy to `path.dots-bak` before overwriting
- `fileutil.EnsureParent(path)`: mkdir -p for the parent directory
- `fileutil.CopyFile(src, dst)`: copy with parent creation
- `shell.InsertBlock(path, content, dryRun)`: marker-delimited block insert/update
- `shell.RemoveBlock(path, dryRun)`: remove marker-delimited block
