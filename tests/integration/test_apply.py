"""Integration tests for full deploy cycle on tmp dirs; idempotency."""

import os
import stat
from pathlib import Path
from unittest.mock import patch

import pytest


# ── Security: path validation ───────────────────────────────────────────────


def test_src_outside_repo_rejected(dots, tmp_repo, tmp_home):
    """Source path that escapes repo root is rejected."""
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "../../../etc/passwd"
dst = "~/.stolen"
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        result = dots.cmd_apply(config)

    assert not (tmp_home / ".stolen").exists()


def test_dst_outside_home_rejected(dots, tmp_repo, tmp_home):
    """Destination path outside $HOME is rejected."""
    (tmp_repo / "files" / "evil").write_text("pwned")
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "files/evil"
dst = "/tmp/pwned"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        result = dots.cmd_apply(config)

    assert not Path("/tmp/pwned").exists()


def test_secret_written_with_restricted_mode(dots, tmp_repo, tmp_home):
    """Decrypted secrets are written with 600 permissions from the start."""
    # We can't easily test the race-free write directly, but we can verify
    # the final permissions on a secret file deployed in copy mode
    (tmp_repo / "files" / "token").write_text("secret-value")
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "files/token"
dst = "~/.token"
mode = "600"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    target = tmp_home / ".token"
    assert target.exists()
    assert stat.S_IMODE(target.stat().st_mode) == 0o600


def test_symlink_created(dots, tmp_repo, tmp_home):
    """Symlink created correctly."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    target = tmp_home / ".vimrc"
    assert target.is_symlink()
    assert target.resolve() == (tmp_repo / "files" / ".vimrc").resolve()


def test_symlink_idempotent(dots, tmp_repo, tmp_home):
    """Correct symlink not recreated (idempotent)."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)
        result1 = os.lstat(str(tmp_home / ".vimrc")).st_ino
        dots.cmd_apply(config)
        result2 = os.lstat(str(tmp_home / ".vimrc")).st_ino

    assert result1 == result2  # Same inode, not recreated


def test_stale_symlink_replaced(dots, tmp_repo, tmp_home):
    """Stale symlink replaced after backup."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # Create stale symlink
    target = tmp_home / ".vimrc"
    target.symlink_to("/nonexistent/old/path")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    assert target.is_symlink()
    assert target.resolve() == (tmp_repo / "files" / ".vimrc").resolve()
    # Backup should exist
    assert (tmp_home / ".vimrc.dots-bak").exists() or (tmp_home / ".vimrc.dots-bak").is_symlink()


def test_existing_file_backed_up(dots, tmp_repo, tmp_home):
    """Existing non-symlink file backed up before replacement."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # Create existing file
    target = tmp_home / ".vimrc"
    target.write_text("old content")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    assert target.is_symlink()
    bak = tmp_home / ".vimrc.dots-bak"
    assert bak.exists()
    assert bak.read_text() == "old content"


def test_ssh_dir_700(dots, tmp_repo, tmp_home):
    """.ssh/ parent created with 700 permissions."""
    (tmp_repo / "files" / ".ssh").mkdir(parents=True)
    (tmp_repo / "files" / ".ssh" / "config").write_text("# ssh config")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    ssh_dir = tmp_home / ".ssh"
    assert ssh_dir.exists()
    assert stat.S_IMODE(ssh_dir.stat().st_mode) == 0o700


def test_copy_mode(dots, tmp_repo, tmp_home):
    """--copy flag forces copy mode."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config, force_copy=True)

    target = tmp_home / ".vimrc"
    assert not target.is_symlink()
    assert target.read_text() == "set nocompatible"


def test_dry_run_no_side_effects(dots, tmp_repo, tmp_home):
    """Dry run produces no side effects."""
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config, dry_run=True)

    assert not (tmp_home / ".vimrc").exists()


def test_mode_applied(dots, tmp_repo, tmp_home):
    """File mode applied after write."""
    (tmp_repo / "files" / ".netrc").write_text("machine example.com")
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[[file]]
src = "files/.netrc"
dst = "~/.netrc"
mode = "600"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(toml, tmp_repo)
        dots.cmd_apply(config)

    target = tmp_home / ".netrc"
    assert target.exists()
    assert stat.S_IMODE(target.stat().st_mode) == 0o600


def test_nested_dirs_created(dots, tmp_repo, tmp_home):
    """Nested target directories are created."""
    (tmp_repo / "files" / ".config" / "nvim").mkdir(parents=True)
    (tmp_repo / "files" / ".config" / "nvim" / "init.lua").write_text("-- nvim")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = dots.load_config(tmp_repo / "dots.toml", tmp_repo)
        dots.cmd_apply(config)

    assert (tmp_home / ".config" / "nvim" / "init.lua").is_symlink()
