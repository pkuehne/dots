"""Integration tests for migrate scan, suggest, --write."""

from unittest.mock import patch

from dots.commands import cmd_migrate
from dots.config import Config


def test_migrate_finds_unmanaged(tmp_repo, tmp_home, capsys):
    (tmp_home / ".zshrc").write_text("# my zshrc")
    (tmp_home / ".gitconfig").write_text("[user]")

    config = Config(repo_root=tmp_repo)
    config.files = []

    cmd_migrate(config)

    output = capsys.readouterr().out
    assert ".zshrc" in output
    assert ".gitconfig" in output


def test_migrate_skips_managed(tmp_repo, tmp_home, capsys):
    (tmp_home / ".zshrc").write_text("# my zshrc")
    (tmp_repo / "files" / ".zshrc").write_text("# managed")

    config = Config(repo_root=tmp_repo)
    config.files = []

    cmd_migrate(config)

    output = capsys.readouterr().out
    # .zshrc should not be suggested since it exists in files/
    assert "Found: ~/.zshrc" not in output


def test_migrate_no_files_found(tmp_repo, tmp_home, capsys):
    config = Config(repo_root=tmp_repo)
    config.files = []

    cmd_migrate(config)

    output = capsys.readouterr().out
    assert "No unmanaged dotfiles found" in output


def test_migrate_write_copies(tmp_repo, tmp_home, capsys):
    (tmp_home / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # .vimrc is not in MIGRATE_SCAN by default, but let's add it
    with patch("dots.commands.MIGRATE_SCAN", [".vimrc"]):
        config = Config(repo_root=tmp_repo)
        config.files = []
        cmd_migrate(config, write=True)

    assert (tmp_repo / "files" / ".vimrc").exists()
    toml_content = (tmp_repo / "dots.toml").read_text()
    assert "[[file]]" in toml_content
