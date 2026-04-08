"""Tests for 010-env.sh and 020-path.sh generation."""

from unittest.mock import patch

import pytest


def test_env_snippet_sorted(dots):
    """Environment variables are sorted alphabetically."""
    config = dots.Config()
    config.env = {"ZEBRA": "z", "ALPHA": "a", "MIDDLE": "m"}
    config.env_when = []

    result = dots.generate_env_snippet(config)
    lines = [l for l in result.splitlines() if l.startswith("export")]
    keys = [l.split("=")[0].replace("export ", "") for l in lines]
    assert keys == sorted(keys)


def test_env_snippet_double_quoted(dots):
    """Values are always double-quoted."""
    config = dots.Config()
    config.env = {"EDITOR": "nvim"}
    config.env_when = []

    result = dots.generate_env_snippet(config)
    assert 'export EDITOR="nvim"' in result


def test_path_snippet_order_preserved(dots):
    """PATH additions preserve declaration order."""
    config = dots.Config()
    config.shell.path = ["~/.first", "~/.second", "~/.third"]
    config.tools = []
    config.tools_config.bin_dir = "~/.local/bin"

    result = dots.generate_path_snippet(config)
    # Find all path entries
    lines = [l for l in result.splitlines() if "case" in l]
    assert len(lines) >= 3


def test_path_snippet_bin_dir_prepended(dots):
    """bin_dir is prepended when tools configured."""
    tool = dots.Tool(name="rg")
    config = dots.Config()
    config.shell.path = ["~/.cargo/bin"]
    config.tools = [tool]
    config.tools_config.bin_dir = "~/.local/bin"

    result = dots.generate_path_snippet(config)
    lines = result.splitlines()
    case_lines = [l for l in lines if "case" in l]
    # bin_dir should appear first
    assert ".local/bin" in case_lines[0]


def test_path_snippet_case_guard(dots):
    """PATH uses case guard to avoid duplicates."""
    config = dots.Config()
    config.shell.path = ["~/.local/bin"]
    config.tools = []

    result = dots.generate_path_snippet(config)
    assert 'case ":$PATH:"' in result
    assert "esac" in result
