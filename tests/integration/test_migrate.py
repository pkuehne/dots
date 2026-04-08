"""Integration tests for migrate scan, suggest, --write."""

import os
from pathlib import Path
from unittest.mock import patch

import pytest


def test_migrate_finds_unmanaged(dots, tmp_repo, tmp_home, capsys):
    """migrate finds unmanaged dotfiles."""
    (tmp_home / ".zshrc").write_text("# my zshrc")
    (tmp_home / ".gitconfig").write_text("[user]")

    config = dots.Config(repo_root=tmp_repo)
    config.files = []

    dots.cmd_migrate(config)

    output = capsys.readouterr().out
    assert ".zshrc" in output
    assert ".gitconfig" in output


def test_migrate_skips_managed(dots, tmp_repo, tmp_home, capsys):
    """migrate skips already managed files."""
    (tmp_home / ".zshrc").write_text("# my zshrc")
    (tmp_repo / "files" / ".zshrc").write_text("# managed")

    config = dots.Config(repo_root=tmp_repo)
    config.files = []

    dots.cmd_migrate(config)

    output = capsys.readouterr().out
    # .zshrc should not be suggested since it exists in files/
    assert "Found: ~/.zshrc" not in output


def test_migrate_no_files_found(dots, tmp_repo, tmp_home, capsys):
    """migrate with no dotfiles shows nothing to migrate."""
    config = dots.Config(repo_root=tmp_repo)
    config.files = []

    dots.cmd_migrate(config)

    output = capsys.readouterr().out
    assert "No unmanaged dotfiles found" in output


def test_migrate_write_copies(dots, tmp_repo, tmp_home, capsys):
    """migrate --write copies files and appends to dots.toml."""
    (tmp_home / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # .vimrc is not in MIGRATE_SCAN by default, but let's add it
    with patch.object(dots, "MIGRATE_SCAN", [".vimrc"]):
        config = dots.Config(repo_root=tmp_repo)
        config.files = []
        dots.cmd_migrate(config, write=True)

    assert (tmp_repo / "files" / ".vimrc").exists()
    toml_content = (tmp_repo / "dots.toml").read_text()
    assert "[[file]]" in toml_content
