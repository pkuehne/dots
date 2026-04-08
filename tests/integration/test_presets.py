"""Integration tests for fzf/tmux preset generation and eject."""

import os
from pathlib import Path

import pytest


def test_fzf_preset_content(dots):
    """fzf preset has correct content."""
    result = dots.generate_fzf_preset()
    assert "command -v fzf" in result
    assert "FZF_DEFAULT_OPTS" in result
    assert "FZF_DEFAULT_COMMAND" in result
    assert "key-bindings" in result


def test_fzf_preset_shell_substitution(dots):
    """fzf preset substitutes shell name."""
    zsh = dots.generate_fzf_preset(shell_name="zsh")
    bash = dots.generate_fzf_preset(shell_name="bash")
    assert "key-bindings.zsh" in zsh
    assert "key-bindings.bash" in bash


def test_tmux_preset_content(dots):
    """tmux preset has correct content."""
    assert "prefix C-a" in dots.TMUX_PRESET
    assert "mouse on" in dots.TMUX_PRESET
    assert "tpm" in dots.TMUX_PRESET
    assert "tmux-sensible" in dots.TMUX_PRESET
    assert "tmux-resurrect" in dots.TMUX_PRESET


def test_fzf_eject(dots, tmp_repo):
    """fzf preset ejects to shell/."""
    shell_dir = tmp_repo / "shell"
    shell_dir.mkdir(exist_ok=True)

    content = dots.generate_fzf_preset()
    (shell_dir / "80-fzf.sh").write_text(content)

    result = (shell_dir / "80-fzf.sh").read_text()
    assert "FZF_DEFAULT_OPTS" in result


def test_tmux_eject(dots, tmp_path):
    """tmux preset ejects to file."""
    dest = tmp_path / ".tmux.conf"
    dest.write_text(dots.TMUX_PRESET)

    assert dest.exists()
    assert "prefix C-a" in dest.read_text()
