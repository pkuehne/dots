# dots — developer guide

## Architecture

See docs/architecture.md for a full overview.

## Commands

```sh
# Run tests
pytest tests/

# Run a specific test file
pytest tests/unit/test_config.py -v

# Lint and format
ruff check src/ tests/
ruff format src/ tests/

# Run e2e tests (requires Docker)
./tests/e2e/run.sh
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

## Key invariants

1. dots is a Python package under src/dots/. Entry point: dots.cli:main.
2. No mandatory third-party imports. All optional deps are guarded.
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

## Common patterns

- expand(path): resolves ~ and $VAR in a path string → Path
- run(cmd): subprocess.run with error handling and good error messages
- idempotent_insert(path, content, marker): marker-delimited block insert/update
- sha256_file(path): content hash for change detection
- backup(path): copies file to path.dots-bak before overwriting
