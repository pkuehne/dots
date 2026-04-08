"""Integration tests for managed.gitconfig write + [include] insertion."""

import os
from pathlib import Path
from unittest.mock import patch

import pytest


def test_git_init_generates_config(dots, tmp_home):
    """dots git init generates managed.gitconfig."""
    config = dots.Config()
    config.git.name = "Test User"
    config.git.email = "test@example.com"
    config.git.editor = "nvim"
    config.tools = []

    git_dir = tmp_home / ".config" / "dots" / "git"
    git_dir.mkdir(parents=True)

    content = dots.generate_gitconfig(config)
    (git_dir / "managed.gitconfig").write_text(content)

    result = (git_dir / "managed.gitconfig").read_text()
    assert "name = Test User" in result
    assert "email = test@example.com" in result


def test_include_inserted_into_gitconfig(dots, tmp_home):
    """[include] inserted into existing .gitconfig."""
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text("[user]\n    name = Existing\n")

    dots.idempotent_insert(gitconfig, dots.GIT_INCLUDE_BLOCK)

    content = gitconfig.read_text()
    assert "[include]" in content
    assert "managed.gitconfig" in content
    assert "[user]" in content  # Original content preserved


def test_include_not_duplicated(dots, tmp_home):
    """[include] not duplicated on second apply."""
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text("[user]\n    name = Test\n")

    dots.idempotent_insert(gitconfig, dots.GIT_INCLUDE_BLOCK)
    dots.idempotent_insert(gitconfig, dots.GIT_INCLUDE_BLOCK)

    content = gitconfig.read_text()
    assert content.count("[include]") == 1


def test_uninit_removes_include(dots, tmp_home):
    """uninit removes [include], leaves rest intact."""
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text(
        "[user]\n    name = Test\n" + dots.GIT_INCLUDE_BLOCK + "\n[core]\n    editor = vim\n"
    )

    dots.remove_marker_block(gitconfig)

    content = gitconfig.read_text()
    assert dots.MARKER_START not in content
    assert "[user]" in content
    assert "[core]" in content
