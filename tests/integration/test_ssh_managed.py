"""Integration tests for SSH config write + Include insertion; permissions."""

import os
import stat
from pathlib import Path
from unittest.mock import patch

import pytest


def test_ssh_init_creates_config(dots, tmp_home):
    """ssh init creates generated config file."""
    config = dots.Config()
    config.ssh_hosts = [
        dots.SSHHost(host="myhost", options={"user": "peter"}),
    ]

    with patch("dots.platform.detect_platform", return_value="linux"):
        dots.ssh_init(config)

    generated = tmp_home / ".config" / "dots" / "ssh" / "config"
    assert generated.exists()
    assert "Host myhost" in generated.read_text()


def test_ssh_config_permissions_600(dots, tmp_home):
    """Generated SSH config is 600."""
    config = dots.Config()
    config.ssh_hosts = []

    with patch("dots.platform.detect_platform", return_value="linux"):
        dots.ssh_init(config)

    generated = tmp_home / ".config" / "dots" / "ssh" / "config"
    assert stat.S_IMODE(generated.stat().st_mode) == 0o600


def test_ssh_dir_700(dots, tmp_home):
    """~/.ssh created with 700."""
    config = dots.Config()
    config.ssh_hosts = []

    with patch("dots.platform.detect_platform", return_value="linux"):
        dots.ssh_init(config)

    ssh_dir = tmp_home / ".ssh"
    assert ssh_dir.exists()
    assert stat.S_IMODE(ssh_dir.stat().st_mode) == 0o700


def test_include_inserted_into_ssh_config(dots, tmp_home):
    """Include prepended to ~/.ssh/config."""
    ssh_dir = tmp_home / ".ssh"
    ssh_dir.mkdir(mode=0o700)
    ssh_config = ssh_dir / "config"
    ssh_config.write_text("Host existing\n    User alice\n")

    config = dots.Config()
    config.ssh_hosts = []

    with patch("dots.platform.detect_platform", return_value="linux"):
        dots.ssh_init(config)

    content = ssh_config.read_text()
    assert content.startswith(dots.SSH_INCLUDE_LINE)
    assert "Host existing" in content


def test_include_not_duplicated(dots, tmp_home):
    """Include not duplicated on second init."""
    config = dots.Config()
    config.ssh_hosts = []

    with patch("dots.platform.detect_platform", return_value="linux"):
        dots.ssh_init(config)
        dots.ssh_init(config)

    ssh_config = tmp_home / ".ssh" / "config"
    content = ssh_config.read_text()
    assert content.count(dots.SSH_INCLUDE_LINE) == 1
