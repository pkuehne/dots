# ADR 011: Package Split

## Status
Accepted (supersedes ADR 001)

## Context
ADR 001 chose a single-file design for maximum portability. As dots grew to ~3000 lines, the monolith became hard to navigate and impossible to install via `pipx`. Users expect `pipx install .` to work for CLI tools, and the single extensionless file could not provide a standard entry point.

## Decision
Split dots into a Python package under `src/dots/` with a `pyproject.toml` (hatchling backend). The package is installable via `pip install -e .` or `pipx install .`. Entry point: `dots.cli:main`.

Module layout:

| Module | Responsibility |
|--------|---------------|
| `constants.py` | Version, markers, skip patterns |
| `errors.py` | Error classes with hints |
| `platform.py` | OS/arch detection |
| `utils.py` | Path helpers, run(), backup() |
| `config.py` | Dataclasses + TOML parsing |
| `discovery.py` | File discovery from files/ dirs |
| `templates.py` | Jinja2 rendering |
| `secrets.py` | age encrypt/decrypt |
| `shell.py` | Shell snippets + bootstrapper |
| `git.py` | Gitconfig generation |
| `ssh.py` | SSH config generation |
| `deploy.py` | File deployment |
| `tools.py` | Tool installation |
| `repos.py` | Git repo management |
| `presets.py` | Presets + login shell files |
| `cli.py` | Argument parser + command dispatch |

## Consequences
- `pipx install .` and `python -m dots` both work
- Each module is small and focused (~50-250 lines)
- `__init__.py` re-exports all public names so existing test patterns work
- Patchable functions use module-level references (`import dots.platform as _plat`) so a single `patch("dots.platform.detect_platform", ...)` works across all modules
- Trade-off: no longer a single file you can `curl | python3`; mitigated by `pipx install git+<repo>`
- No mandatory dependencies; optional deps (tomli, jinja2) remain guarded
