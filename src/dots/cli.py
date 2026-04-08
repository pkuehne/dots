"""CLI argument parser, commands, and main dispatch."""

from __future__ import annotations

import argparse
import difflib
import os
import re
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path
from typing import List, Optional

from dots.config import Config, load_config
from dots.constants import (
    GENERATED_HEADER,
    MARKER_START,
    SENSITIVE_DIRS,
    VERSION,
)
from dots.deploy import deploy_file, get_file_state, matches_platform
from dots.discovery import discover_files, merge_file_entries
from dots.errors import DotsError
from dots.git import GIT_INCLUDE_BLOCK, generate_gitconfig
import dots.platform as _plat
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
from dots.tools import (
    find_install_method,
    install_tool,
    tool_is_installed,
)
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
        print("  ✓ {}".format(msg))

    def warn(msg):
        nonlocal warnings
        warnings += 1
        print("  ⚠ {}".format(msg))

    def fail(msg):
        nonlocal errors
        errors += 1
        print("  ✗ {}".format(msg))

    print("dots doctor")
    print()

    # Python version
    v = sys.version_info
    if v >= (3, 8):
        ok("Python {}.{}.{}".format(v.major, v.minor, v.micro))
    else:
        fail("Python {}.{}.{} < 3.8 required".format(v.major, v.minor, v.micro))

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
            ok("Shell bootstrapper installed in {}".format(zshrc))
        else:
            warn("Shell bootstrapper not found in {}".format(zshrc))

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
                ok("~/{} permissions: {:o}".format(dirname, actual))
            else:
                warn(
                    "~/{} permissions: {:o} (expected {:o})".format(
                        dirname, actual, expected_mode
                    )
                )

    # Disk space
    try:
        st = os.statvfs(str(Path.home()))
        free_mb = (st.f_bavail * st.f_frsize) // (1024 * 1024)
        if free_mb < 100:
            warn("Low disk space in $HOME: {} MB free".format(free_mb))
        else:
            ok("Disk space: {} MB free".format(free_mb))
    except (OSError, AttributeError):
        pass

    print()
    if errors:
        print("{} error(s), {} warning(s)".format(errors, warnings))
        return 1
    if warnings:
        print("{} warning(s)".format(warnings))
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
        # Check if already managed
        candidate_src = "files/{}".format(rel)
        if candidate_src in managed_srcs:
            continue
        if (repo_root / candidate_src).exists():
            continue

        if fp.is_symlink():
            target = fp.resolve()
            if str(target).startswith(str(repo_root)):
                print("  ✓ ~/{} — already symlinked into repo".format(rel))
                continue

        dest_dir = "files.d/{}/".format(plat) if plat else "files/"
        print("  Found: ~/{}".format(rel))
        print("    Suggested [[file]] entry:")
        print('    src  = "{}{}"\n    dst  = "~/{}"'.format(dest_dir, rel, rel))
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
            print("  Copied ~/{} → {}".format(rel, repo_dest.relative_to(repo_root)))

        # Append to dots.toml
        toml_path = repo_root / "dots.toml"
        with open(str(toml_path), "a") as f:
            f.write("\n# Migrated files\n")
            for rel, dest_dir in suggestions:
                f.write("\n[[file]]\n")
                f.write('src = "{}{}"\n'.format(dest_dir, rel))
                f.write('dst = "~/{}"\n'.format(rel))
        print("\n  Entries appended to dots.toml")


# ── CLI Commands ────────────────────────────────────────────────────────────

def find_repo_root(start: Path = None) -> Optional[Path]:
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
            "dots.toml already exists in {}".format(d),
            hint="Remove it first if you want to re-initialize.",
        )

    d.mkdir(parents=True, exist_ok=True)
    (d / "files").mkdir(exist_ok=True)
    (d / "files.d").mkdir(exist_ok=True)
    (d / "shell").mkdir(exist_ok=True)

    # Minimal dots.toml
    (d / "dots.toml").write_text(textwrap.dedent("""\
        [meta]
        version = 1

        [shell]
        managed = false

        [git]
        managed = false
    """))

    print("✓ Initialized dots in {}".format(d))
    print("  Created: dots.toml, files/, files.d/, shell/")


def cmd_apply(
    config: Config,
    file_args: List[str] = None,
    dry_run: bool = False,
    force_copy: bool = False,
) -> int:
    plat = _plat.detect_platform()
    repo_root = config.repo_root
    errors = 0

    # 1. Deploy files
    discovered = discover_files(repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    if file_args:
        # Filter to requested files
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
                dst_display = "~" + dst_display[len(str(Path.home())):]
            print("  {:4s}  {}".format(result, dst_display))
        except DotsError as e:
            print(e.render())
            errors += 1

    # 2-5. Shell snippets (if managed)
    if config.shell.managed and not file_args:
        shell_dir = expand(config.shell.dir)
        shell_dir.mkdir(parents=True, exist_ok=True)

        print("\nGenerating shell snippets...")

        # 010-env.sh
        env_content = generate_env_snippet(config)
        env_path = shell_dir / "010-env.sh"
        if dry_run:
            print("  WRITE  {}".format(env_path))
        else:
            env_path.write_text(env_content)
            print("  OK     010-env.sh")

        # 020-path.sh
        path_content = generate_path_snippet(config)
        path_path = shell_dir / "020-path.sh"
        if dry_run:
            print("  WRITE  {}".format(path_path))
        else:
            path_path.write_text(path_content)
            print("  OK     020-path.sh")

        # User snippets from shell/
        shell_src = repo_root / "shell"
        if shell_src.is_dir():
            for f in sorted(shell_src.iterdir()):
                if not f.is_file():
                    continue
                name = f.name
                # Validate prefix range
                match = re.match(r"^(\d+)", name)
                if match:
                    prefix = int(match.group(1))
                    valid_ranges = [(30, 49), (80, 89), (90, 99)]
                    in_range = any(lo <= prefix <= hi for lo, hi in valid_ranges)
                    if not in_range:
                        print(
                            "  ⚠ Warning: {} has prefix {} outside expected ranges "
                            "(030-049, 080-089, 090+)".format(name, prefix)
                        )

                dst = shell_dir / name
                if name.endswith(".j2"):
                    rendered = render_template(f, config)
                    out_name = name[:-3]
                    dst = shell_dir / out_name
                    if dry_run:
                        print("  RENDER {}".format(out_name))
                    else:
                        dst.write_text(rendered)
                        print("  OK     {}".format(out_name))
                else:
                    if dry_run:
                        print("  DEPLOY {}".format(name))
                    else:
                        shutil.copy2(str(f), str(dst))
                        print("  OK     {}".format(name))

        # Per-tool snippets (050-*)
        for tool in config.tools:
            if not tool.shell.env and not tool.shell.init:
                continue
            snippet = generate_tool_snippet(tool)
            snippet_name = "050-{}.sh".format(tool.name)
            snippet_path = shell_dir / snippet_name
            if dry_run:
                print("  WRITE  {}".format(snippet_name))
            else:
                snippet_path.write_text(snippet)
                print("  OK     {}".format(snippet_name))

        # 000-custom.sh
        custom = generate_custom_snippet(repo_root)
        if custom:
            custom_path = shell_dir / "000-custom.sh"
            if dry_run:
                print("  WRITE  000-custom.sh")
            else:
                custom_path.write_text(custom)
                print("  OK     000-custom.sh")

    # 6. Git managed
    if config.git.managed and not file_args:
        print("\nGenerating git config...")
        git_dir = expand("~/.config/dots/git")
        git_dir.mkdir(parents=True, exist_ok=True)
        gitconfig_content = generate_gitconfig(config)
        gitconfig_path = git_dir / "managed.gitconfig"
        if dry_run:
            print("  WRITE  {}".format(gitconfig_path))
        else:
            gitconfig_path.write_text(gitconfig_content)
            print("  OK     managed.gitconfig")
            # Insert include
            home_gitconfig = expand("~/.gitconfig")
            changed = idempotent_insert(home_gitconfig, GIT_INCLUDE_BLOCK)
            if changed:
                print("  OK     [include] added to ~/.gitconfig")

    # 7. SSH managed
    if config.ssh.managed and not file_args:
        print("\nGenerating SSH config...")
        if dry_run:
            print("  WRITE  ~/.config/dots/ssh/config")
        else:
            ssh_init(config)
            print("  OK     SSH config generated")

    # 8. Clone missing repos
    if config.repos and not file_args:
        print("\nCloning repos...")
        for r in config.repos:
            if not matches_platform(r.only, plat):
                print("  SKIP   {} (platform)".format(r.name))
                continue
            if r.profile and r.profile != config.active_profile:
                print("  SKIP   {} (profile)".format(r.name))
                continue
            try:
                if dry_run:
                    dst = expand(r.dst)
                    if dst.exists():
                        print("  OK     {} (already cloned)".format(r.name))
                    else:
                        print("  CLONE  {}".format(r.name))
                else:
                    result = clone_repo(r)
                    if result == "already":
                        print("  OK     {} (already cloned)".format(r.name))
                    else:
                        print("  OK     {} cloned".format(r.name))
            except DotsError as e:
                print(e.render())
                errors += 1

    # Presets
    if not file_args:
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

    # Login shell files
    if config.shell.managed and config.shell.login and not file_args:
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

    if errors:
        return 3
    return 0


def cmd_status(config: Config) -> None:
    plat = _plat.detect_platform()
    discovered = discover_files(config.repo_root, plat)
    all_files = merge_file_entries(discovered, config.files)

    print("Files:")
    for entry in all_files:
        state = get_file_state(entry, config)
        dst = entry.dst
        if dst.startswith(str(Path.home())):
            dst = "~" + dst[len(str(Path.home())):]
        print("  {:4s}  {}".format(state, dst))

    if config.shell.managed:
        print("\nShell:")
        shell_dir = expand(config.shell.dir)
        if shell_dir.is_dir():
            for f in sorted(shell_dir.iterdir()):
                print("  OK    {}".format(f.name))
        zshrc = expand(config.shell.zshrc)
        if zshrc.exists() and MARKER_START in zshrc.read_text():
            print("  ✓ Bootstrapper installed in {}".format(config.shell.zshrc))
        else:
            print("  ✗ Bootstrapper not found in {}".format(config.shell.zshrc))

    if config.repos:
        print("\nRepos:")
        for r in config.repos:
            dst = expand(r.dst)
            if dst.exists():
                if (dst / ".git").exists():
                    print("  OK    {} → {}".format(r.name, r.dst))
                else:
                    print("  ✗     {} — not a git repo".format(r.name))
            else:
                print("  MISS  {}".format(r.name))


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
        "File not found: {}".format(file_arg),
        hint="Check dots list for available files.",
    )


def cmd_add(config: Config, path: str, dest: str = "") -> None:
    src = Path(path).resolve()
    if not src.exists():
        raise DotsError("File not found: {}".format(path))

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
        dst_str = "~" + dst_str[len(home):]

    # Append to dots.toml
    toml_path = repo_root / "dots.toml"
    with open(str(toml_path), "a") as f:
        f.write("\n[[file]]\n")
        f.write('src = "{}"\n'.format(rel_src))
        f.write('dst = "{}"\n'.format(dst_str))

    print("✓ Adopted {} → {}".format(path, rel_src))
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
            dst = "~" + dst[len(str(Path.home())):]
        print("  {:4s}  {}".format(state, dst))


def cmd_encrypt(config: Config, file_path: str, output: str = "") -> None:
    src = Path(file_path)
    if not src.exists():
        raise DotsError("File not found: {}".format(file_path))
    out = Path(output) if output else src.with_suffix(src.suffix + ".age")
    encrypt_file(src, config.secrets.recipient, out)
    print("✓ Encrypted {} → {}".format(src, out))


def cmd_decrypt(config: Config, file_path: str, output: str = "") -> None:
    src = Path(file_path)
    if not src.exists():
        raise DotsError("File not found: {}".format(file_path))
    if not str(src).endswith(".age"):
        raise DotsError("File must end in .age: {}".format(file_path))
    identity = expand(config.secrets.identity)
    data = decrypt_file(src, identity)
    if output:
        out = Path(output)
    else:
        out = src.with_suffix("")
    out.write_bytes(data)
    print("✓ Decrypted {} → {}".format(src, out))


# ── CLI Argument Parser ─────────────────────────────────────────────────────

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dots",
        description="Dotfile management, tool installation, and shell environment generation.",
    )
    parser.add_argument("--version", action="version", version="dots {}".format(VERSION))
    parser.add_argument("--profile", default="", help="Activate a named profile")
    parser.add_argument(
        "--repo", default="", help="Path to dotfiles repository root"
    )

    sub = parser.add_subparsers(dest="command")

    # init
    p_init = sub.add_parser("init", help="Scaffold a new dots repository")
    p_init.add_argument("dir", nargs="?", default=".", help="Directory to initialize")

    # apply
    p_apply = sub.add_parser("apply", help="Deploy files and generate configs")
    p_apply.add_argument("files", nargs="*", help="Specific files to deploy")
    p_apply.add_argument(
        "-n", "--dry-run", action="store_true", help="Print actions without executing"
    )
    p_apply.add_argument(
        "-c", "--copy", action="store_true", help="Force copy mode"
    )
    p_apply.add_argument("--profile", dest="apply_profile", default="")

    # preview
    p_preview = sub.add_parser("preview", help="Alias for apply --dry-run")
    p_preview.add_argument("files", nargs="*")

    # status
    sub.add_parser("status", help="Show deployment state")

    # diff
    p_diff = sub.add_parser("diff", help="Show diffs between source and deployed")
    p_diff.add_argument("file", nargs="?", default="")

    # edit
    p_edit = sub.add_parser("edit", help="Open source file in editor")
    p_edit.add_argument("file", help="File to edit")

    # add
    p_add = sub.add_parser("add", help="Adopt an existing file into the repo")
    p_add.add_argument("path", help="Path to adopt")
    p_add.add_argument("--dest", default="", help="Override repo destination")

    # list
    p_list = sub.add_parser("list", help="List managed files")
    p_list.add_argument("--all", action="store_true", help="Include skipped files")

    # doctor
    sub.add_parser("doctor", help="System health check")

    # migrate
    p_migrate = sub.add_parser("migrate", help="Scan for unmanaged dotfiles")
    p_migrate.add_argument("--write", action="store_true", help="Copy and add entries")
    p_migrate.add_argument("--platform", default="", help="Target platform dir")

    # encrypt
    p_encrypt = sub.add_parser("encrypt", help="Encrypt a file with age")
    p_encrypt.add_argument("file", help="File to encrypt")
    p_encrypt.add_argument("-o", "--output", default="", help="Output path")

    # decrypt
    p_decrypt = sub.add_parser("decrypt", help="Decrypt an .age file")
    p_decrypt.add_argument("file", help="File to decrypt")
    p_decrypt.add_argument("-o", "--output", default="", help="Output path")

    # ── tools ──
    p_tools = sub.add_parser("tools", help="Manage tool installations")
    tools_sub = p_tools.add_subparsers(dest="tools_command")

    p_tools_check = tools_sub.add_parser("check", help="Check installed tools")
    p_tools_check.add_argument("names", nargs="*")
    p_tools_check.add_argument("--tag", default="")

    p_tools_install = tools_sub.add_parser("install", help="Install missing tools")
    p_tools_install.add_argument("names", nargs="*")
    p_tools_install.add_argument("--tag", default="")
    p_tools_install.add_argument("-n", "--dry-run", action="store_true")
    p_tools_install.add_argument("-f", "--force", action="store_true")

    p_tools_list = tools_sub.add_parser("list", help="List configured tools")
    p_tools_list.add_argument("--tag", default="")

    # ── shell ──
    p_shell = sub.add_parser("shell", help="Manage shell integration")
    shell_sub = p_shell.add_subparsers(dest="shell_command")

    p_shell_init = shell_sub.add_parser("init", help="Install bootstrapper")
    p_shell_init.add_argument("--shells", nargs="*", default=["zsh", "bash"])
    p_shell_init.add_argument("-n", "--dry-run", action="store_true")

    p_shell_uninit = shell_sub.add_parser("uninit", help="Remove bootstrapper")
    p_shell_uninit.add_argument("--shells", nargs="*", default=["zsh", "bash"])

    shell_sub.add_parser("check", help="Show bootstrapper status")

    p_shell_show = shell_sub.add_parser("show", help="Print generated snippets")
    p_shell_show.add_argument("--assembled", action="store_true")

    p_shell_clean = shell_sub.add_parser("clean", help="Remove stale snippets")
    p_shell_clean.add_argument("-n", "--dry-run", action="store_true")

    # ── repos ──
    p_repos = sub.add_parser("repos", help="Manage git repositories")
    repos_sub = p_repos.add_subparsers(dest="repos_command")

    p_repos_clone = repos_sub.add_parser("clone", help="Clone missing repos")
    p_repos_clone.add_argument("names", nargs="*")

    p_repos_update = repos_sub.add_parser("update", help="Update cloned repos")
    p_repos_update.add_argument("names", nargs="*")

    repos_sub.add_parser("status", help="Show repo states")

    # ── git ──
    p_git = sub.add_parser("git", help="Manage git config")
    git_sub = p_git.add_subparsers(dest="git_command")

    p_git_init = git_sub.add_parser("init", help="Enable git managed mode")
    p_git_init.add_argument("-n", "--dry-run", action="store_true")

    git_sub.add_parser("show", help="Print managed.gitconfig")
    git_sub.add_parser("uninit", help="Remove dots [include]")

    # ── ssh ──
    p_ssh = sub.add_parser("ssh", help="Manage SSH config")
    ssh_sub = p_ssh.add_subparsers(dest="ssh_command")

    p_ssh_init = ssh_sub.add_parser("init", help="Enable SSH managed mode")
    p_ssh_init.add_argument("-n", "--dry-run", action="store_true")

    ssh_sub.add_parser("show", help="Print SSH config")
    ssh_sub.add_parser("uninit", help="Remove dots Include")

    # ── env ──
    p_env = sub.add_parser("env", help="Manage environment variables")
    env_sub = p_env.add_subparsers(dest="env_command")

    env_sub.add_parser("show", help="Print 010-env.sh content")
    env_sub.add_parser("check", help="Check [[env.when]] conditions")

    # ── presets ──
    p_presets = sub.add_parser("presets", help="Manage presets")
    presets_sub = p_presets.add_subparsers(dest="presets_command")

    p_presets_show = presets_sub.add_parser("show", help="Print preset output")
    p_presets_show.add_argument("preset", help="Preset name")

    p_presets_eject = presets_sub.add_parser("eject", help="Eject preset to files")
    p_presets_eject.add_argument("preset", help="Preset name")
    p_presets_eject.add_argument("--dest", default="", help="Output directory")

    return parser


# ── Main Dispatch ───────────────────────────────────────────────────────────

def main(argv: List[str] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 0

    try:
        # Commands that don't need config
        if args.command == "init":
            cmd_init(args.dir)
            return 0

        # Find repo root
        repo_path = args.repo if args.repo else None
        if repo_path:
            repo_root = Path(repo_path).resolve()
        else:
            repo_root = find_repo_root()

        if repo_root is None:
            raise DotsError(
                "No dots.toml found",
                hint="Searched from: {} (walked up to /)\n\n"
                     "Hints:\n"
                     "· Run from your dotfiles directory, or:\n"
                     "· Create a new dots.toml:  dots init\n"
                     "· Specify the repo:        dots --repo ~/dotfiles apply\n"
                     "· Set the env var:         export DOTS_REPO=~/dotfiles".format(
                    Path.cwd()
                ),
            )

        # Check for DOTS_REPO env var
        if repo_root is None:
            env_repo = os.environ.get("DOTS_REPO")
            if env_repo:
                repo_root = Path(env_repo).resolve()

        toml_path = repo_root / "dots.toml" if repo_root else None
        profile = getattr(args, "apply_profile", "") or args.profile
        config = load_config(
            toml_path=toml_path,
            repo_root=repo_root,
            profile=profile,
        )

        # Dispatch
        if args.command == "apply":
            return cmd_apply(
                config,
                file_args=args.files,
                dry_run=args.dry_run,
                force_copy=args.copy,
            )

        elif args.command == "preview":
            return cmd_apply(config, file_args=args.files, dry_run=True)

        elif args.command == "status":
            cmd_status(config)
            return 0

        elif args.command == "diff":
            cmd_diff(config, file_arg=args.file)
            return 0

        elif args.command == "edit":
            cmd_edit(config, file_arg=args.file)
            return 0

        elif args.command == "add":
            cmd_add(config, path=args.path, dest=args.dest)
            return 0

        elif args.command == "list":
            cmd_list(config, show_all=args.all)
            return 0

        elif args.command == "doctor":
            return cmd_doctor(config)

        elif args.command == "migrate":
            cmd_migrate(config, write=args.write, plat=args.platform)
            return 0

        elif args.command == "encrypt":
            cmd_encrypt(config, file_path=args.file, output=args.output)
            return 0

        elif args.command == "decrypt":
            cmd_decrypt(config, file_path=args.file, output=args.output)
            return 0

        elif args.command == "tools":
            return dispatch_tools(config, args)

        elif args.command == "shell":
            return dispatch_shell(config, args)

        elif args.command == "repos":
            return dispatch_repos(config, args)

        elif args.command == "git":
            return dispatch_git(config, args)

        elif args.command == "ssh":
            return dispatch_ssh(config, args)

        elif args.command == "env":
            return dispatch_env(config, args)

        elif args.command == "presets":
            return dispatch_presets(config, args)

        else:
            parser.print_help()
            return 0

    except DotsError as e:
        print(e.render(), file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130
    except Exception as e:
        print(
            "\n✗ Unexpected error: {}\n\n"
            "  This is a bug. Please report it with the full traceback:\n"
            "  {}".format(e, e),
            file=sys.stderr,
        )
        import traceback
        traceback.print_exc(file=sys.stderr)
        return 1


# ── Subcommand Dispatchers ──────────────────────────────────────────────────

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
            print("  {} {}{}".format(
                status, t.name,
                " — {}".format(t.desc) if t.desc else "",
            ))
        return 0

    elif args.tools_command == "install":
        tools = filter_tools(args.names, args.tag)
        errors = 0
        for t in tools:
            if not args.force and tool_is_installed(t):
                print("  ✓ {} (already installed)".format(t.name))
                continue
            inst = find_install_method(t)
            if not inst:
                print("  ✗ {} — no suitable install method".format(t.name))
                errors += 1
                continue
            if args.dry_run:
                print("  → {} via {}".format(t.name, inst.method))
                continue
            try:
                print("  Installing {} via {}...".format(t.name, inst.method))
                method = install_tool(t, inst, bin_dir)
                print("  ✓ {} installed via {}".format(t.name, method))
            except DotsError as e:
                print(e.render())
                errors += 1
        return 3 if errors else 0

    elif args.tools_command == "list":
        tools = filter_tools([], args.tag)
        for t in tools:
            tags = " [{}]".format(", ".join(t.tags)) if t.tags else ""
            methods = ", ".join(i.method for i in t.install)
            print("  {} — {}{}  ({})".format(t.name, t.desc, tags, methods))
        return 0

    return 0


def dispatch_shell(config: Config, args) -> int:
    if args.shell_command == "init":
        shell_dir = expand(config.shell.dir)
        shell_dir.mkdir(parents=True, exist_ok=True)

        for shell_name in args.shells:
            if shell_name == "zsh":
                path = expand(config.shell.zshrc)
                content = ZSH_BOOTSTRAPPER
            elif shell_name == "bash":
                path = expand(config.shell.bashrc)
                content = BASH_BOOTSTRAPPER
            else:
                continue

            if args.dry_run:
                print("  Would install bootstrapper in {}".format(path))
            else:
                changed = idempotent_insert(path, content)
                if changed:
                    print("  ✓ Bootstrapper installed in {}".format(path))
                else:
                    print("  ✓ Bootstrapper already up-to-date in {}".format(path))
        return 0

    elif args.shell_command == "uninit":
        for shell_name in args.shells:
            if shell_name == "zsh":
                path = expand(config.shell.zshrc)
            elif shell_name == "bash":
                path = expand(config.shell.bashrc)
            else:
                continue
            removed = remove_marker_block(path)
            if removed:
                print("  ✓ Bootstrapper removed from {}".format(path))
            else:
                print("  — No bootstrapper found in {}".format(path))
        return 0

    elif args.shell_command == "check":
        for shell_name, rc_path in [("zsh", config.shell.zshrc), ("bash", config.shell.bashrc)]:
            path = expand(rc_path)
            if path.exists() and MARKER_START in path.read_text():
                print("  ✓ {} bootstrapper installed".format(shell_name))
            else:
                print("  ✗ {} bootstrapper not found".format(shell_name))
        return 0

    elif args.shell_command == "show":
        if args.assembled:
            # Print all snippets in order
            shell_dir = expand(config.shell.dir)
            if shell_dir.is_dir():
                for f in sorted(shell_dir.iterdir()):
                    if f.is_file():
                        print("# === {} ===".format(f.name))
                        print(f.read_text())
        else:
            print("# 010-env.sh")
            print(generate_env_snippet(config))
            print("# 020-path.sh")
            print(generate_path_snippet(config))
            for tool in config.tools:
                if tool.shell.env or tool.shell.init:
                    print("# 050-{}.sh".format(tool.name))
                    print(generate_tool_snippet(tool))
        return 0

    elif args.shell_command == "clean":
        shell_dir = expand(config.shell.dir)
        if not shell_dir.is_dir():
            return 0
        # Determine expected snippet names
        expected = {"010-env.sh", "020-path.sh"}
        for tool in config.tools:
            if tool.shell.env or tool.shell.init:
                expected.add("050-{}.sh".format(tool.name))
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
                    print("  Would remove {}".format(f.name))
                else:
                    f.unlink()
                    print("  ✓ Removed {}".format(f.name))
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
                    print("  ✓ {} (already cloned)".format(r.name))
                else:
                    print("  ✓ {} cloned".format(r.name))
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
                    print("  ✗ {} not cloned".format(r.name))
                else:
                    print("  ✓ {} updated".format(r.name))
            except DotsError as e:
                print(e.render())
                errors += 1
        return 3 if errors else 0

    elif args.repos_command == "status":
        for r in config.repos:
            dst = expand(r.dst)
            if not dst.exists():
                print("  MISS  {}".format(r.name))
            elif not (dst / ".git").exists():
                print("  ✗     {} — not a git repo".format(r.name))
            else:
                # Check dirty
                try:
                    result = subprocess.run(
                        ["git", "status", "--porcelain"],
                        cwd=str(dst),
                        capture_output=True,
                        text=True,
                    )
                    if result.stdout.strip():
                        print("  DIRTY {}".format(r.name))
                    else:
                        print("  OK    {}".format(r.name))
                except Exception:
                    print("  ✗     {} — git error".format(r.name))
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
        ssh_config = expand("~/.ssh/config")
        if ssh_config.exists():
            text = ssh_config.read_text()
            if SSH_INCLUDE_LINE in text:
                new_text = text.replace(SSH_INCLUDE_LINE + "\n\n", "")
                new_text = new_text.replace(SSH_INCLUDE_LINE + "\n", "")
                new_text = new_text.replace(SSH_INCLUDE_LINE, "")
                ssh_config.write_text(new_text)
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
                reasons.append("platform {}".format(plat))
            if ew.if_tool and not shutil.which(ew.if_tool):
                active = False
                reasons.append("{} not found".format(ew.if_tool))
            status = "✓" if active else "✗"
            extra = " ({})".format(", ".join(reasons)) if reasons else ""
            print("  {} {}={}{}".format(status, ew.key, ew.value, extra))
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
                "Unknown preset: {}".format(args.preset),
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
            print("✓ Ejected fzf preset to {}".format(dest_dir / "80-fzf.sh"))
            print("  Set presets.fzf = false in dots.toml and customise freely.")
        elif args.preset == "tmux":
            dest = Path(args.dest) if args.dest else expand("~/.tmux.conf")
            if dest.is_dir():
                dest = dest / ".tmux.conf"
            dest.write_text(TMUX_PRESET)
            print("✓ Ejected tmux preset to {}".format(dest))
            print("  Set presets.tmux = false in dots.toml and customise freely.")
        else:
            raise DotsError(
                "Unknown preset: {}".format(args.preset),
                hint="Available presets: fzf, tmux",
            )
        return 0

    return 0
