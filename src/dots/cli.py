"""CLI argument parser and main dispatch."""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

from dots.commands import (
    cmd_add,
    cmd_apply,
    cmd_decrypt,
    cmd_diff,
    cmd_doctor,
    cmd_edit,
    cmd_encrypt,
    cmd_init,
    cmd_list,
    cmd_migrate,
    cmd_status,
    dispatch_env,
    dispatch_git,
    dispatch_presets,
    dispatch_repos,
    dispatch_shell,
    dispatch_ssh,
    dispatch_tools,
    find_repo_root,
)
from dots.config import load_config
from dots.constants import VERSION
from dots.errors import DotsError


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dots",
        description="Dotfile management, tool installation, and shell environment generation.",
    )
    parser.add_argument("--version", action="version", version=f"dots {VERSION}")
    parser.add_argument("--profile", default="", help="Activate a named profile")
    parser.add_argument("--repo", default="", help="Path to dotfiles repository root")

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
    p_apply.add_argument("-c", "--copy", action="store_true", help="Force copy mode")
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


def main(argv: list[str] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 0

    try:
        if args.command == "init":
            cmd_init(args.dir)
            return 0

        # Find repo root: explicit flag > walk up > env var
        repo_path = args.repo if args.repo else None
        if repo_path:
            repo_root = Path(repo_path).resolve()
        else:
            repo_root = find_repo_root()

        if repo_root is None:
            env_repo = os.environ.get("DOTS_REPO")
            if env_repo:
                repo_root = Path(env_repo).resolve()

        if repo_root is None:
            raise DotsError(
                "No dots.toml found",
                hint=f"Searched from: {Path.cwd()} (walked up to /)\n\n"
                "Hints:\n"
                "· Run from your dotfiles directory, or:\n"
                "· Create a new dots.toml:  dots init\n"
                "· Specify the repo:        dots --repo ~/dotfiles apply\n"
                "· Set the env var:         export DOTS_REPO=~/dotfiles",
            )

        toml_path = repo_root / "dots.toml" if repo_root else None
        profile = getattr(args, "apply_profile", "") or args.profile
        config = load_config(
            toml_path=toml_path,
            repo_root=repo_root,
            profile=profile,
        )

        dispatch = {
            "apply": lambda: cmd_apply(
                config,
                file_args=args.files,
                dry_run=args.dry_run,
                force_copy=args.copy,
            ),
            "preview": lambda: cmd_apply(config, file_args=args.files, dry_run=True),
            "status": lambda: (cmd_status(config), 0)[1],
            "diff": lambda: (cmd_diff(config, file_arg=args.file), 0)[1],
            "edit": lambda: (cmd_edit(config, file_arg=args.file), 0)[1],
            "add": lambda: (cmd_add(config, path=args.path, dest=args.dest), 0)[1],
            "list": lambda: (cmd_list(config, show_all=args.all), 0)[1],
            "doctor": lambda: cmd_doctor(config),
            "migrate": lambda: (cmd_migrate(config, write=args.write, plat=args.platform), 0)[1],
            "encrypt": lambda: (cmd_encrypt(config, file_path=args.file, output=args.output), 0)[1],
            "decrypt": lambda: (cmd_decrypt(config, file_path=args.file, output=args.output), 0)[1],
            "tools": lambda: dispatch_tools(config, args),
            "shell": lambda: dispatch_shell(config, args),
            "repos": lambda: dispatch_repos(config, args),
            "git": lambda: dispatch_git(config, args),
            "ssh": lambda: dispatch_ssh(config, args),
            "env": lambda: dispatch_env(config, args),
            "presets": lambda: dispatch_presets(config, args),
        }

        handler = dispatch.get(args.command)
        if handler:
            return handler()

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
            f"\n✗ Unexpected error: {e}\n\n"
            "  This is a bug. Please report it with the full traceback:\n"
            f"  {e}",
            file=sys.stderr,
        )
        import traceback

        traceback.print_exc(file=sys.stderr)
        return 1
