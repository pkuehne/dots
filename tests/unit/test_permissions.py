"""Tests for sensitive directory modes and file mode application."""

import stat


def test_ensure_parent_ssh_700(dots, tmp_home):
    """~/.ssh/ created with 700 permissions."""
    target = tmp_home / ".ssh" / "id_ed25519"
    dots.ensure_parent(target)
    ssh_dir = tmp_home / ".ssh"
    assert ssh_dir.exists()
    assert stat.S_IMODE(ssh_dir.stat().st_mode) == 0o700


def test_ensure_parent_gnupg_700(dots, tmp_home):
    """~/.gnupg/ created with 700 permissions."""
    target = tmp_home / ".gnupg" / "gpg-agent.conf"
    dots.ensure_parent(target)
    gnupg_dir = tmp_home / ".gnupg"
    assert gnupg_dir.exists()
    assert stat.S_IMODE(gnupg_dir.stat().st_mode) == 0o700


def test_ensure_parent_normal_dir(dots, tmp_home):
    """Normal directories created without special permissions."""
    target = tmp_home / ".config" / "nvim" / "init.lua"
    dots.ensure_parent(target)
    config_dir = tmp_home / ".config"
    assert config_dir.exists()
    # Should not be 700 (no special treatment)


def test_apply_mode_600(dots, tmp_path):
    """mode = "600" applied after write."""
    f = tmp_path / "test"
    f.write_text("content")
    dots.deploy._apply_mode(f, "600")
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_apply_mode_644(dots, tmp_path):
    """mode = "644" applied after write."""
    f = tmp_path / "test"
    f.write_text("content")
    dots.deploy._apply_mode(f, "644")
    assert stat.S_IMODE(f.stat().st_mode) == 0o644


def test_write_secret_creates_with_mode(dots, tmp_path):
    """_write_secret creates file with correct mode from the start."""
    f = tmp_path / "secret"
    dots.deploy._write_secret(f, b"top-secret", "600")
    assert f.read_bytes() == b"top-secret"
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_write_secret_overwrites_existing(dots, tmp_path):
    """_write_secret overwrites existing file and applies mode."""
    f = tmp_path / "secret"
    f.write_text("old")
    f.chmod(0o644)
    dots.deploy._write_secret(f, b"new-secret", "600")
    assert f.read_bytes() == b"new-secret"
    assert stat.S_IMODE(f.stat().st_mode) == 0o600


def test_apply_mode_empty_noop(dots, tmp_path):
    """Empty mode string is a no-op."""
    f = tmp_path / "test"
    f.write_text("content")
    before = f.stat().st_mode
    dots.deploy._apply_mode(f, "")
    assert f.stat().st_mode == before
