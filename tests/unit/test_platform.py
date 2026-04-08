"""Tests for platform and architecture detection."""

import os
from unittest.mock import patch

import pytest


def test_detect_termux(dots):
    """Termux detected via /data/data/com.termux."""
    with patch("os.path.isdir", return_value=True):
        assert dots.detect_platform() == "termux"


def test_detect_linux(dots):
    """Linux detected on non-Termux Linux."""
    with patch("os.path.isdir", return_value=False), \
         patch("platform.system", return_value="Linux"):
        assert dots.detect_platform() == "linux"


def test_detect_darwin(dots):
    """macOS detected."""
    with patch("os.path.isdir", return_value=False), \
         patch("platform.system", return_value="Darwin"):
        assert dots.detect_platform() == "darwin"


def test_detect_windows(dots):
    """Windows detected."""
    with patch("os.path.isdir", return_value=False), \
         patch("platform.system", return_value="Windows"):
        assert dots.detect_platform() == "windows"


def test_detect_arch_x86_64(dots):
    with patch("platform.machine", return_value="x86_64"):
        assert dots.detect_arch() == "x86_64"


def test_detect_arch_aarch64(dots):
    with patch("platform.machine", return_value="aarch64"):
        assert dots.detect_arch() == "aarch64"


def test_detect_arch_arm64(dots):
    with patch("platform.machine", return_value="arm64"):
        assert dots.detect_arch() == "aarch64"


def test_detect_goarch_amd64(dots):
    with patch("platform.machine", return_value="x86_64"):
        assert dots.detect_goarch() == "amd64"


def test_detect_goarch_arm64(dots):
    with patch("platform.machine", return_value="aarch64"):
        assert dots.detect_goarch() == "arm64"


def test_detect_os_name_linux(dots):
    with patch.object(dots, "detect_platform", return_value="linux"):
        assert dots.detect_os_name() == "linux"


def test_detect_os_name_termux(dots):
    """Termux reports as linux for OS name."""
    with patch.object(dots, "detect_platform", return_value="termux"):
        assert dots.detect_os_name() == "linux"


def test_detect_os_name_darwin(dots):
    with patch.object(dots, "detect_platform", return_value="darwin"):
        assert dots.detect_os_name() == "darwin"
