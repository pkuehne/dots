"""Command implementations for dots CLI."""

from __future__ import annotations

import difflib
import os
import re
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

import dots.platform as _plat
from dots.config import Config
from dots.constants import MARKER_START, SENSITIVE_DIRS
from dots.deploy import deploy_file, get_file_state, matches_platform
from dots.discovery import discover_files, merge_file_entries
from dots.errors import DotsError
from dots.git import GIT_INCLUDE_BLOCK, generate_gitconfig
from dots.presets import (
    TMUX_PRESET,
    generate_fzf_preset,
    generate_profile,
    generate_zprofile,
)
from dots.repos import clone_repo, update_repo
from dots.secrets import decrypt_file, encrypt_file
from dots.shell import (
    BASH_BOOTSTRAPPER,
    ZSH_BOOTSTRAPPER,
    generate_custom_snippet,
    generate_env_snippet,
    generate_path_snippet,
    generate_tool_snippet,
    idempotent_insert,
    remove_marker_block,
)
from dots.ssh import SSH_INCLUDE_LINE, generate_ssh_config, ssh_init
from dots.templates import render_template
from dots.tools import find_install_method, install_tool, tool_is_installed
from dots.utils import backup, expand

try:
    import jinja2  # type: ignore[import-untyped]
except ImportError:
    jinja2 = None  # type: ignore[assignment]


# ── Doctor ──────────────────────────────────────────────────────────────────


def cmd_doctor(config: Config) -> int:
    warnings = 0
    errors = 0

    def ok(msg):
        print(f"  ✓ {msg}")

    def warn(msg):
        nonlocal warnings
        warnings += 1
        print(f"  ⚠ {msg}")

    def fail(msg):
        nonlocal errors
        errors += 1
        print(f"  ✗ {msg}")

    print("dots doctor")
    print()

    # Python version
    v = sys.version_info
    if v >= (3, 10):
        ok(f"Python {v.major}.{v.minor}.{v.micro}")
    else:
        fail(f"Python {v.major}.{v.minor}.{v.micro} — 3.10+ required")

    # dots.toml
    toml_path = config.repo_root / "dots.toml"
    if toml_path.exists():
        ok("dots.toml found and parsed")
    else:
        warn("dots.toml not found (zero-config mode)")

    # Jinja2
    j2_files = list(config.repo_root.glob("**/*.j2"))
    if j2_files:
        if jinja2:
            ok("jinja2 installed (.j2 files present)")
        else:
            fail("jinja2 not installed but .j2 files present")

    # Git
    repos = config.repos
    if repos:
        if shutil.which("git"):
            ok("git available (repos configured)")
        else:
            fail("git not found but [[repo]] entries configured")

    # Age
    age_files = list(config.repo_root.glob("**/*.age"))
    has_recipient = bool(config.secrets.recipient)
    if age_files or has_recipient:
        if shutil.which("age"):
            ok("age available (secrets configured)")
        else:
            fail("age not found but .age files or [secrets] configured")

    # GITHUB_TOKEN
    if config.tools:
        if os.environ.get("GITHUB_TOKEN"):
            ok("GITHUB_TOKEN set")
        else:
            warn("GITHUB_TOKEN not set — GitHub API rate limits apply (60 req/hr)")

    # ~/.local/bin on PATH
    local_bin = str(expand("~/.local/bin"))
    if local_bin in os.environ.get("PATH", ""):
        ok("~/.local/bin on PATH")
    else:
        warn("~/.local/bin not on PATH")

    # Shell bootstrapper
    if config.shell.managed:
        zshrc = expand(config.shell.zshrc)
        if zshrc.exists() and MARKER_START in zshrc.read_text():
            ok(f"Shell bootstrapper installed in {zshrc}")
        else:
            warn(f"Shell bootstrapper not found in {zshrc}")

    # Git include
    if config.git.managed:
        gitconfig = expand("~/.gitconfig")
        if gitconfig.exists() and MARKER_START in gitconfig.read_text():
            ok("Git [include] present in ~/.gitconfig")
        else:
            warn("Git [include] not found in ~/.gitconfig")

    # SSH include
    if config.ssh.managed:
        ssh_config = expand("~/.ssh/config")
        if ssh_config.exists() and SSH_INCLUDE_LINE in ssh_config.read_text():
            ok("SSH Include present in ~/.ssh/config")
        else:
            warn("SSH Include not found in ~/.ssh/config")

    # Sensitive dir permissions
    for dirname, expected_mode in SENSITIVE_DIRS.items():
        p = Path.home() / dirname
        if p.exists():
            actual = p.stat().st_mode & 0o777
            if actual == expected_mode:
                ok(f"~/{dirname} permissions: {actual:o}")
            else:
                warn(f"~/{dirname} permissions: {actual:o} (expected {expected_mode:o})")

    # Disk space
    try:
        st = os.statvfs(str(Path.home()))
        free_mb = (st.f_bavail * st.f_frsize) // (1024 * 1024)
        if free_mb < 100:
            warn(f"Low disk space in $HOME: {free_mb} MB free")
        else:
            ok(f"Disk space: {free_mb} MB free")
    except (OSError, AttributeError):
        pass

    print()
    if errors:
        print(f"{errors} error(s), {warnings} warning(s)")
        return 1
    if warnings:
        print(f"{warnings} warning(s)")
        return 1
    print("All checks passed")
    return 0


# ── Migrate ─────────────────────────────────────────────────────────────────

MIGRATE_SCAN = [
    ".zshrc",
    ".bashrc",
    ".gitconfig",
    ".vimrc",
    ".tmux.conf",
    ".ssh/config",
    ".config/nvim/init.lua",
    ".config/starship.toml",
    ".config/alacritty/alacritty.yml",
    ".config/alacritty/alacritty.toml",
    ".config/kitty/kitty.conf",
    ".config/wezterm/wezterm.lua",
]


def cmd_migrate(config: Config, write: bool = False, plat: str = "") -> None:
    home = Path.home()
    repo_root = config.repo_root
    managed_srcs = {e.src for e in config.files}
    suggestions = []

    for rel in MIGRATE_SCAN:
        fp = home / rel
        if not fp.exists():
            continue
        candidate_src = f"files/{rel}"
        if candidate_src in managed_srcs:
            continue
        if (repo_root / candidate_src).exists():
            continue

        if fp.is_symlink():
            target = fp.resolve()
            if str(target).startswith(str(repo_root)):
                print(f"  ✓ ~/{rel} — already symlinked into repo")
                continue

        dest_dir = f"files.d/{plat}/" if plat else "files/"
        print(f"  Found: ~/{rel}")
        print("    Suggested [[file]] entry:")
        print(f'    src  = "{dest_dir}{rel}"\n    dst  = "~/{rel}"')
        print()
        suggestions.append((rel, dest_dir))

    if not suggestions:
        print("  No unmanaged dotfiles found to migrate.")
        return

    if write:
        for rel, dest_dir in suggestions:
            src_path = home / rel
            repo_dest = repo_root / dest_dir / rel
            repo_dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(str(src_path), str(repo_dest))
            print(f"  Copied ~/{rel} → {repo_dest.relative_to(repo_root)}")

        toml_path = repo_root / "dots.toml"
        with open(str(toml_path), "a") as f:
            f.write("\n# Migrated files\n")
            for rel, dest_dir in suggestions:
                f.write("\n[[file]]\n")
                f.write(f'src = "{dest_dir}{rel}"\n')
                f.write(f'dst = "~/{rel}"\n')
        print("\n  Entries appended to dots.toml")


# ── Commands ────────────────────────────────────────────────────────────────


def find_repo_root(start: Path = None) -> Path | None:
    if start is None:
        start = Path.cwd()
    current = start.resolve()
    while True:
        if (current / "dots.toml").exists():
            return current
        if (current / "files").is_dir():
            return current
        parent = current.parent
        if parent == current:
            break
        current = parent
    return None


def cmd_init(directory: str = ".") -> None:
    d = Path(directory).resolve()
    if (d / "dots.toml").exists():
        raise DotsError(
            f"dots.toml already exists in {d}",
            hint="Remove it first if you want to re-initialize.",
        )

    d.mkdir(parents=True, exist_ok=True)
    (d / "files").mkdir(exist_ok=True)
    (d / "files.d").mkdir(exist_ok=True)
    (d / "shell").mkdir(exist_ok=True)

    (d / "dots.toml").write_text(
        textwrap.dedent("""\
        [meta]
        version = 1

        [shell]
        managed = false

        [git]
        managed = false
    """)
    )

    print(f"✓ Initialized dots in {d}")
    print("  Created: dots.toml, files/, files.d/, shell/")


def cmd_apply(
    config: Config,
    file_args: list[str] = None,
    dry_run: bool = False,
    force_copy: bool = False,
) -> int:
    plat = _plat.detect_platform()
    repo_root = config.repo_root
    errors = 0

    discovered = discover_files(repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    if file_args:
        filtered = []
        for entry in all_files:
            for arg in file_args:
                if arg in entry.src or arg in entry.dst or entry.src.endswith(arg):
                    filtered.append(entry)
                    break
        all_files = filtered

    print("Deploying files...")
    for entry in all_files:
        try:
            result = deploy_file(entry, config, dry_run=dry_run, force_copy=force_copy)
            dst_display = entry.dst
            if dst_display.startswith(str(Path.home())):
                dst_display = "~" + dst_display[len(str(Path.home())) :]
            print(f"  {result:4s}  {dst_display}")
        except DotsError as e:
            print(e.render())
            errors += 1

    if config.shell.managed and not file_args:
        _apply_shell(config, repo_root, dry_run)

    if config.git.managed and not file_args:
        _apply_git(config, dry_run)

    if config.ssh.managed and not file_args:
        print("\nGenerating SSH config...")
        if dry_run:
            print("  WRITE  ~/.config/dots/ssh/config")
        else:
            ssh_init(config)
            print("  OK     SSH config generated")

    if config.repos and not file_args:
        errors += _apply_repos(config, plat, dry_run)

    if not file_args:
        _apply_presets(config, dry_run)

    if config.tools and not file_args:
        errors += _apply_tools(config, dry_run)

    if config.shell.managed and config.shell.login and not file_args:
        _apply_login_shell(config, dry_run)

    if errors:
        return 3
    return 0


def _apply_shell(config: Config, repo_root: Path, dry_run: bool) -> None:
    shell_dir = expand(config.shell.dir)
    shell_dir.mkdir(parents=True, exist_ok=True)

    print("\nGenerating shell snippets...")

    env_content = generate_env_snippet(config)
    env_path = shell_dir / "010-env.sh"
    if dry_run:
        print(f"  WRITE  {env_path}")
    else:
        env_path.write_text(env_content)
        print("  OK     010-env.sh")

    path_content = generate_path_snippet(config)
    path_path = shell_dir / "020-path.sh"
    if dry_run:
        print(f"  WRITE  {path_path}")
    else:
        path_path.write_text(path_content)
        print("  OK     020-path.sh")

    shell_src = repo_root / "shell"
    if shell_src.is_dir():
        for f in sorted(shell_src.iterdir()):
            if not f.is_file():
                continue
            name = f.name
            match = re.match(r"^(\d+)", name)
            if match:
                prefix = int(match.group(1))
                valid_ranges = [(30, 49), (80, 89), (90, 99)]
                in_range = any(lo <= prefix <= hi for lo, hi in valid_ranges)
                if not in_range:
                    print(
                        f"  ⚠ Warning: {name} has prefix {prefix} outside expected ranges "
                        "(030-049, 080-089, 090+)"
                    )

            dst = shell_dir / name
            if name.endswith(".j2"):
                rendered = render_template(f, config)
                out_name = name[:-3]
                dst = shell_dir / out_name
                if dry_run:
                    print(f"  RENDER {out_name}")
                else:
                    dst.write_text(rendered)
                    print(f"  OK     {out_name}")
            else:
                if dry_run:
                    print(f"  DEPLOY {name}")
                else:
                    shutil.copy2(str(f), str(dst))
                    print(f"  OK     {name}")

    for tool in config.tools:
        if not tool.shell.env and not tool.shell.init:
            continue
        snippet = generate_tool_snippet(tool)
        snippet_name = f"050-{tool.name}.sh"
        snippet_path = shell_dir / snippet_name
        if dry_run:
            print(f"  WRITE  {snippet_name}")
        else:
            snippet_path.write_text(snippet)
            print(f"  OK     {snippet_name}")

    custom = generate_custom_snippet(repo_root)
    if custom:
        custom_path = shell_dir / "000-custom.sh"
        if dry_run:
            print("  WRITE  000-custom.sh")
        else:
            custom_path.write_text(custom)
            print("  OK     000-custom.sh")

    # Install bootstrapper into shell rc files
    for shell_name, rc_setting, bootstrapper in [
        ("zsh", config.shell.zshrc, ZSH_BOOTSTRAPPER),
        ("bash", config.shell.bashrc, BASH_BOOTSTRAPPER),
    ]:
        rc_path = expand(rc_setting)
        if dry_run:
            print(f"  WRITE  bootstrapper in {rc_path}")
        else:
            changed = idempotent_insert(rc_path, bootstrapper)
            if changed:
                print(f"  OK     bootstrapper installed in {rc_path}")


def _apply_git(config: Config, dry_run: bool) -> None:
    print("\nGenerating git config...")
    git_dir = expand("~/.config/dots/git")
    git_dir.mkdir(parents=True, exist_ok=True)
    gitconfig_content = generate_gitconfig(config)
    gitconfig_path = git_dir / "managed.gitconfig"
    if dry_run:
        print(f"  WRITE  {gitconfig_path}")
    else:
        gitconfig_path.write_text(gitconfig_content)
        print("  OK     managed.gitconfig")
        home_gitconfig = expand("~/.gitconfig")
        changed = idempotent_insert(home_gitconfig, GIT_INCLUDE_BLOCK)
        if changed:
            print("  OK     [include] added to ~/.gitconfig")


def _apply_repos(config: Config, plat: str, dry_run: bool) -> int:
    errors = 0
    print("\nCloning repos...")
    for r in config.repos:
        if not matches_platform(r.only, plat):
            print(f"  SKIP   {r.name} (platform)")
            continue
        if r.profile and r.profile != config.active_profile:
            print(f"  SKIP   {r.name} (profile)")
            continue
        try:
            if dry_run:
                dst = expand(r.dst)
                if dst.exists():
                    print(f"  OK     {r.name} (already cloned)")
                else:
                    print(f"  CLONE  {r.name}")
            else:
                result = clone_repo(r)
                if result == "already":
                    print(f"  OK     {r.name} (already cloned)")
                else:
                    print(f"  OK     {r.name} cloned")
        except DotsError as e:
            print(e.render())
            errors += 1
    return errors


def _apply_presets(config: Config, dry_run: bool) -> None:
    if config.presets.fzf:
        print("\nGenerating fzf preset...")
        if config.shell.managed:
            shell_dir = expand(config.shell.dir)
            fzf_path = shell_dir / "050-fzf-preset.sh"
            content = generate_fzf_preset()
            if dry_run:
                print("  WRITE  050-fzf-preset.sh")
            else:
                fzf_path.write_text(content)
                print("  OK     050-fzf-preset.sh")

    if config.presets.tmux:
        print("\nGenerating tmux preset...")
        tmux_path = expand("~/.tmux.conf")
        if dry_run:
            print("  WRITE  ~/.tmux.conf")
        else:
            if tmux_path.exists() and MARKER_START not in tmux_path.read_text():
                backup(tmux_path)
                print("  ⚠ Backed up existing ~/.tmux.conf")
            tmux_path.write_text(TMUX_PRESET)
            print("  OK     ~/.tmux.conf")


def _apply_login_shell(config: Config, dry_run: bool) -> None:
    print("\nGenerating login shell files...")
    zprofile = generate_zprofile(config)
    profile = generate_profile(config)
    zp = expand("~/.zprofile")
    pp = expand("~/.profile")
    if dry_run:
        print("  WRITE  ~/.zprofile")
        print("  WRITE  ~/.profile")
    else:
        zp.write_text(zprofile)
        pp.write_text(profile)
        print("  OK     ~/.zprofile")
        print("  OK     ~/.profile")


def cmd_status(config: Config) -> None:
    plat = _plat.detect_platform()
    discovered = discover_files(config.repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    print("Files:")
    for entry in all_files:
        state = get_file_state(entry, config)
        dst = entry.dst
        if dst.startswith(str(Path.home())):
            dst = "~" + dst[len(str(Path.home())) :]
        print(f"  {state:4s}  {dst}")

    if config.shell.managed:
        print("\nShell:")
        shell_dir = expand(config.shell.dir)
        if shell_dir.is_dir():
            for f in sorted(shell_dir.iterdir()):
                print(f"  OK    {f.name}")
        zshrc = expand(config.shell.zshrc)
        if zshrc.exists() and MARKER_START in zshrc.read_text():
            print(f"  ✓ Bootstrapper installed in {config.shell.zshrc}")
        else:
            print(f"  ✗ Bootstrapper not found in {config.shell.zshrc}")

    if config.repos:
        print("\nRepos:")
        for r in config.repos:
            dst = expand(r.dst)
            if dst.exists():
                if (dst / ".git").exists():
                    print(f"  OK    {r.name} → {r.dst}")
                else:
                    print(f"  ✗     {r.name} — not a git repo")
            else:
                print(f"  MISS  {r.name}")


def cmd_diff(config: Config, file_arg: str = "") -> None:
    plat = _plat.detect_platform()
    discovered = discover_files(config.repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    for entry in all_files:
        if file_arg and file_arg not in entry.src and file_arg not in entry.dst:
            continue
        state = get_file_state(entry, config)
        if state != "DIFF" and file_arg == "":
            continue

        src = config.repo_root / entry.src
        dst = Path(expand(entry.dst)) if "~" in entry.dst or "$" in entry.dst else Path(entry.dst)

        if not src.exists() or not dst.exists():
            continue

        src_lines = src.read_text().splitlines(keepends=True)
        dst_lines = dst.read_text().splitlines(keepends=True)
        diff = difflib.unified_diff(
            src_lines,
            dst_lines,
            fromfile="repo: " + entry.src,
            tofile="deployed: " + entry.dst,
        )
        sys.stdout.writelines(diff)


def cmd_edit(config: Config, file_arg: str) -> None:
    plat = _plat.detect_platform()
    discovered = discover_files(config.repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    for entry in all_files:
        if (
            file_arg in entry.src
            or file_arg in entry.dst
            or entry.src.endswith(file_arg)
            or Path(entry.dst).name == file_arg
        ):
            src = config.repo_root / entry.src
            editor = os.environ.get("EDITOR") or os.environ.get("VISUAL") or "vi"
            os.execvp(editor, [editor, str(src)])
            return

    raise DotsError(
        f"File not found: {file_arg}",
        hint="Check dots list for available files.",
    )


def cmd_add(config: Config, path: str, dest: str = "") -> None:
    src = Path(path).resolve()
    if not src.exists():
        raise DotsError(f"File not found: {path}")

    repo_root = config.repo_root
    if dest:
        repo_dest = repo_root / dest
    else:
        repo_dest = repo_root / "files" / src.name

    repo_dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(str(src), str(repo_dest))

    rel_src = str(repo_dest.relative_to(repo_root))
    home = str(Path.home())
    dst_str = str(src)
    if dst_str.startswith(home):
        dst_str = "~" + dst_str[len(home) :]

    toml_path = repo_root / "dots.toml"
    with open(str(toml_path), "a") as f:
        f.write("\n[[file]]\n")
        f.write(f'src = "{rel_src}"\n')
        f.write(f'dst = "{dst_str}"\n')

    print(f"✓ Adopted {path} → {rel_src}")
    print("  [[file]] entry added to dots.toml")


def cmd_list(config: Config, show_all: bool = False) -> None:
    plat = _plat.detect_platform()
    discovered = discover_files(config.repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    for entry in all_files:
        state = get_file_state(entry, config)
        if state == "SKIP" and not show_all:
            continue
        dst = entry.dst
        if dst.startswith(str(Path.home())):
            dst = "~" + dst[len(str(Path.home())) :]
        print(f"  {state:4s}  {dst}")


def cmd_encrypt(config: Config, file_path: str, output: str = "") -> None:
    src = Path(file_path)
    if not src.exists():
        raise DotsError(f"File not found: {file_path}")
    out = Path(output) if output else src.with_suffix(src.suffix + ".age")
    encrypt_file(src, config.secrets.recipient, out)
    print(f"✓ Encrypted {src} → {out}")


def cmd_decrypt(config: Config, file_path: str, output: str = "") -> None:
    src = Path(file_path)
    if not src.exists():
        raise DotsError(f"File not found: {file_path}")
    if not str(src).endswith(".age"):
        raise DotsError(f"File must end in .age: {file_path}")
    identity = expand(config.secrets.identity)
    data = decrypt_file(src, identity)
    if output:
        out = Path(output)
    else:
        out = src.with_suffix("")
    out.write_bytes(data)
    print(f"✓ Decrypted {src} → {out}")


def _apply_tools(config: Config, dry_run: bool) -> int:
    plat = _plat.detect_platform()
    bin_dir = expand(config.tools_config.bin_dir)
    tools = [t for t in config.tools if matches_platform(t.only, plat)]
    if not tools:
        return 0
    print("\nInstalling tools...")
    errors = 0
    for t in tools:
        if tool_is_installed(t):
            print(f"  ✓ {t.name} (already installed)")
            continue
        inst = find_install_method(t)
        if not inst:
            print(f"  ✗ {t.name} — no suitable install method")
            errors += 1
            continue
        if dry_run:
            print(f"  → {t.name} via {inst.method}")
            continue
        try:
            print(f"  Installing {t.name} via {inst.method}...")
            method = install_tool(t, inst, bin_dir)
            print(f"  ✓ {t.name} installed via {method}")
        except DotsError as e:
            print(e.render())
            errors += 1
    return errors


# ── Subcommand dispatchers ──────────────────────────────────────────────────


def dispatch_tools(config: Config, args) -> int:
    plat = _plat.detect_platform()
    bin_dir = expand(config.tools_config.bin_dir)

    def filter_tools(names, tag):
        tools = config.tools
        if names:
            tools = [t for t in tools if t.name in names]
        if tag:
            tools = [t for t in tools if tag in t.tags]
        return [t for t in tools if matches_platform(t.only, plat)]

    if args.tools_command == "check":
        tools = filter_tools(args.names, args.tag)
        for t in tools:
            installed = tool_is_installed(t)
            status = "✓" if installed else "✗"
            print(f"  {status} {t.name}{f' — {t.desc}' if t.desc else ''}")
        return 0

    elif args.tools_command == "install":
        tools = filter_tools(args.names, args.tag)
        errors = 0
        for t in tools:
            if not args.force and tool_is_installed(t):
                print(f"  ✓ {t.name} (already installed)")
                continue
            inst = find_install_method(t)
            if not inst:
                print(f"  ✗ {t.name} — no suitable install method")
                errors += 1
                continue
            if args.dry_run:
                print(f"  → {t.name} via {inst.method}")
                continue
            try:
                print(f"  Installing {t.name} via {inst.method}...")
                method = install_tool(t, inst, bin_dir)
                print(f"  ✓ {t.name} installed via {method}")
            except DotsError as e:
                print(e.render())
                errors += 1
        return 3 if errors else 0

    elif args.tools_command == "list":
        tools = filter_tools([], args.tag)
        for t in tools:
            tags = f" [{', '.join(t.tags)}]" if t.tags else ""
            methods = ", ".join(i.method for i in t.install)
            print(f"  {t.name} — {t.desc}{tags}  ({methods})")
        return 0

    return 0


def dispatch_shell(config: Config, args) -> int:
    if args.shell_command == "show":
        if args.assembled:
            shell_dir = expand(config.shell.dir)
            if shell_dir.is_dir():
                for f in sorted(shell_dir.iterdir()):
                    if f.is_file():
                        print(f"# === {f.name} ===")
                        print(f.read_text())
        else:
            print("# 010-env.sh")
            print(generate_env_snippet(config))
            print("# 020-path.sh")
            print(generate_path_snippet(config))
            for tool in config.tools:
                if tool.shell.env or tool.shell.init:
                    print(f"# 050-{tool.name}.sh")
                    print(generate_tool_snippet(tool))
        return 0

    elif args.shell_command == "clean":
        shell_dir = expand(config.shell.dir)
        if not shell_dir.is_dir():
            return 0
        expected = {"010-env.sh", "020-path.sh"}
        for tool in config.tools:
            if tool.shell.env or tool.shell.init:
                expected.add(f"050-{tool.name}.sh")
        shell_src = config.repo_root / "shell"
        if shell_src.is_dir():
            for f in shell_src.iterdir():
                name = f.name
                if name.endswith(".j2"):
                    name = name[:-3]
                expected.add(name)
        if config.presets.fzf:
            expected.add("050-fzf-preset.sh")

        for f in sorted(shell_dir.iterdir()):
            if f.is_file() and f.name not in expected:
                if args.dry_run:
                    print(f"  Would remove {f.name}")
                else:
                    f.unlink()
                    print(f"  ✓ Removed {f.name}")
        return 0

    return 0


def dispatch_repos(config: Config, args) -> int:
    plat = _plat.detect_platform()

    if args.repos_command == "clone":
        errors = 0
        for r in config.repos:
            if args.names and r.name not in args.names:
                continue
            if not matches_platform(r.only, plat):
                continue
            try:
                result = clone_repo(r)
                if result == "already":
                    print(f"  ✓ {r.name} (already cloned)")
                else:
                    print(f"  ✓ {r.name} cloned")
            except DotsError as e:
                print(e.render())
                errors += 1
        return 3 if errors else 0

    elif args.repos_command == "update":
        errors = 0
        for r in config.repos:
            if args.names and r.name not in args.names:
                continue
            if not matches_platform(r.only, plat):
                continue
            try:
                result = update_repo(r)
                if result == "missing":
                    print(f"  ✗ {r.name} not cloned")
                else:
                    print(f"  ✓ {r.name} updated")
            except DotsError as e:
                print(e.render())
                errors += 1
        return 3 if errors else 0

    elif args.repos_command == "status":
        for r in config.repos:
            dst = expand(r.dst)
            if not dst.exists():
                print(f"  MISS  {r.name}")
            elif not (dst / ".git").exists():
                print(f"  ✗     {r.name} — not a git repo")
            else:
                try:
                    result = subprocess.run(
                        ["git", "status", "--porcelain"],
                        cwd=str(dst),
                        capture_output=True,
                        text=True,
                    )
                    if result.stdout.strip():
                        print(f"  DIRTY {r.name}")
                    else:
                        print(f"  OK    {r.name}")
                except Exception:
                    print(f"  ✗     {r.name} — git error")
        return 0

    return 0


def dispatch_git(config: Config, args) -> int:
    if args.git_command == "init":
        git_dir = expand("~/.config/dots/git")
        git_dir.mkdir(parents=True, exist_ok=True)
        content = generate_gitconfig(config)
        if args.dry_run:
            print(content)
            return 0
        (git_dir / "managed.gitconfig").write_text(content)
        home_gitconfig = expand("~/.gitconfig")
        idempotent_insert(home_gitconfig, GIT_INCLUDE_BLOCK)
        print("✓ Git managed mode enabled")
        print("  Generated: ~/.config/dots/git/managed.gitconfig")
        print("  [include] added to ~/.gitconfig")
        return 0

    elif args.git_command == "show":
        print(generate_gitconfig(config))
        return 0

    elif args.git_command == "uninit":
        home_gitconfig = expand("~/.gitconfig")
        removed = remove_marker_block(home_gitconfig)
        if removed:
            print("✓ Dots [include] removed from ~/.gitconfig")
        else:
            print("— No dots [include] found in ~/.gitconfig")
        return 0

    return 0


def dispatch_ssh(config: Config, args) -> int:
    if args.ssh_command == "init":
        if args.dry_run:
            print(generate_ssh_config(config))
            return 0
        ssh_init(config)
        print("✓ SSH managed mode enabled")
        return 0

    elif args.ssh_command == "show":
        print(generate_ssh_config(config))
        return 0

    elif args.ssh_command == "uninit":
        ssh_config_path = expand("~/.ssh/config")
        if ssh_config_path.exists():
            text = ssh_config_path.read_text()
            if SSH_INCLUDE_LINE in text:
                new_text = text.replace(SSH_INCLUDE_LINE + "\n\n", "")
                new_text = new_text.replace(SSH_INCLUDE_LINE + "\n", "")
                new_text = new_text.replace(SSH_INCLUDE_LINE, "")
                ssh_config_path.write_text(new_text)
                print("✓ Dots Include removed from ~/.ssh/config")
            else:
                print("— No dots Include found in ~/.ssh/config")
        return 0

    return 0


def dispatch_env(config: Config, args) -> int:
    if args.env_command == "show":
        print(generate_env_snippet(config))
        return 0

    elif args.env_command == "check":
        plat = _plat.detect_platform()
        for ew in config.env_when:
            active = True
            reasons = []
            if ew.only and plat not in ew.only:
                active = False
                reasons.append(f"platform {plat}")
            if ew.if_tool and not shutil.which(ew.if_tool):
                active = False
                reasons.append(f"{ew.if_tool} not found")
            status = "✓" if active else "✗"
            extra = f" ({', '.join(reasons)})" if reasons else ""
            print(f"  {status} {ew.key}={ew.value}{extra}")
        return 0

    return 0


def dispatch_presets(config: Config, args) -> int:
    if args.presets_command == "show":
        if args.preset == "fzf":
            print(generate_fzf_preset())
        elif args.preset == "tmux":
            print(TMUX_PRESET)
        else:
            raise DotsError(
                f"Unknown preset: {args.preset}",
                hint="Available presets: fzf, tmux",
            )
        return 0

    elif args.presets_command == "eject":
        repo_root = config.repo_root
        if args.preset == "fzf":
            dest_dir = Path(args.dest) if args.dest else repo_root / "shell"
            dest_dir.mkdir(parents=True, exist_ok=True)
            content = generate_fzf_preset()
            (dest_dir / "80-fzf.sh").write_text(content)
            print(f"✓ Ejected fzf preset to {dest_dir / '80-fzf.sh'}")
            print("  Set presets.fzf = false in dots.toml and customise freely.")
        elif args.preset == "tmux":
            dest = Path(args.dest) if args.dest else expand("~/.tmux.conf")
            if dest.is_dir():
                dest = dest / ".tmux.conf"
            dest.write_text(TMUX_PRESET)
            print(f"✓ Ejected tmux preset to {dest}")
            print("  Set presets.tmux = false in dots.toml and customise freely.")
        else:
            raise DotsError(
                f"Unknown preset: {args.preset}",
                hint="Available presets: fzf, tmux",
            )
        return 0

    return 0
