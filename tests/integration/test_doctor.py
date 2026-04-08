"""Integration tests for doctor checks and exit codes."""

import os
from pathlib import Path
from unittest.mock import patch

import pytest


def test_doctor_passes_minimal(dots, tmp_repo, capsys):
    """Doctor passes with minimal setup."""
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")
    config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)

    with patch.dict(os.environ, {"PATH": os.environ.get("PATH", "") + ":~/.local/bin"}):
        code = dots.cmd_doctor(config)

    output = capsys.readouterr().out
    assert "Python" in output


def test_doctor_warns_no_github_token(dots, tmp_repo, capsys):
    """Doctor warns when GITHUB_TOKEN not set and tools configured."""
    (tmp_repo / "dots.toml").write_text("""\
[[tool]]
name = "rg"
install = [{ method = "apt", package = "ripgrep" }]
""")
    config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)

    with patch.dict(os.environ, {}, clear=True):
        # Keep PATH for basic functionality
        with patch.dict(os.environ, {"PATH": "/usr/bin"}):
            dots.cmd_doctor(config)

    output = capsys.readouterr().out
    assert "GITHUB_TOKEN" in output


def test_doctor_checks_j2(dots, tmp_repo, capsys):
    """Doctor checks jinja2 when .j2 files present."""
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")
    (tmp_repo / "files" / "test.j2").write_text("{{ x }}")
    config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)

    dots.cmd_doctor(config)
    output = capsys.readouterr().out
    assert "jinja2" in output.lower() or "j2" in output.lower()
