"""Tests for sensitive directory modes and file mode application."""

import stat

from dots.deploy import _apply_mode, _write_secret
from dots.utils import ensure_parent


def test_ensure_parent_ssh_700(tmp_home):
    target = tmp_home / ".ssh" / "id_ed25519"
    ensure_parent(target)
    ssh_dir = tmp_home / ".ssh"
    assert ssh_dir.exists()
    assert stat.S_IMODE(ssh_dir.stat().st_mode) == 0o700


def test_ensure_parent_gnupg_700(tmp_home):
    target = tmp_home / ".gnupg" / "gpg-agent.conf"
    ensure_parent(target)
    gnupg_dir = tmp_home / ".gnupg"
    assert gnupg_dir.exists()
    assert stat.S_IMODE(gnupg_dir.stat().st_mode) == 0o700


def test_ensure_parent_normal_dir(tmp_home):
    target = tmp_home / ".config" / "nvim" / "init.lua"
    ensure_parent(target)
    config_dir = tmp_home / ".config"
    assert config_dir.exists()
    # Should not be 700 (no special treatment)


def test_apply_mode_600(tmp_path):
    f = tmp_path / "test"
    f.write_text("content")
    _apply_mode(f, "600")
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_apply_mode_644(tmp_path):
    f = tmp_path / "test"
    f.write_text("content")
    _apply_mode(f, "644")
    assert stat.S_IMODE(f.stat().st_mode) == 0o644


def test_write_secret_creates_with_mode(tmp_path):
    f = tmp_path / "secret"
    _write_secret(f, b"top-secret", "600")
    assert f.read_bytes() == b"top-secret"
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_write_secret_overwrites_existing(tmp_path):
    f = tmp_path / "secret"
    f.write_text("old")
    f.chmod(0o644)
    _write_secret(f, b"new-secret", "600")
    assert f.read_bytes() == b"new-secret"
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_apply_mode_empty_noop(tmp_path):
    f = tmp_path / "test"
    f.write_text("content")
    before = f.stat().st_mode
    _apply_mode(f, "")
    assert f.stat().st_mode == before
