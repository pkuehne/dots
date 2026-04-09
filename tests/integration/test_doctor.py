"""Integration tests for doctor checks and exit codes."""

import os
from unittest.mock import patch

from dots.commands import cmd_doctor
from dots.config import load_config


def test_doctor_passes_minimal(tmp_repo, capsys):
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")
    config = load_config(tmp_repo / "dots.toml", tmp_repo)

    with patch.dict(os.environ, {"PATH": os.environ.get("PATH", "") + ":~/.local/bin"}):
        cmd_doctor(config)

    output = capsys.readouterr().out
    assert "Python" in output


def test_doctor_warns_no_github_token(tmp_repo, capsys):
    (tmp_repo / "dots.toml").write_text("""\
[[tool]]
name = "rg"
install = [{ method = "apt", package = "ripgrep" }]
""")
    config = load_config(tmp_repo / "dots.toml", tmp_repo)

    with patch.dict(os.environ, {}, clear=True):
        # Keep PATH for basic functionality
        with patch.dict(os.environ, {"PATH": "/usr/bin"}):
            cmd_doctor(config)

    output = capsys.readouterr().out
    assert "GITHUB_TOKEN" in output


def test_doctor_checks_j2(tmp_repo, capsys):
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")
    (tmp_repo / "files" / "test.j2").write_text("{{ x }}")
    config = load_config(tmp_repo / "dots.toml", tmp_repo)

    cmd_doctor(config)
    output = capsys.readouterr().out
    assert "jinja2" in output.lower() or "j2" in output.lower()
