"""Tests for platform and architecture detection."""

from unittest.mock import patch

from dots.platform import detect_arch, detect_goarch, detect_os_name, detect_platform


def test_detect_termux():
    with patch("os.path.isdir", return_value=True):
        assert detect_platform() == "termux"


def test_detect_linux():
    with patch("os.path.isdir", return_value=False), patch("platform.system", return_value="Linux"):
        assert detect_platform() == "linux"


def test_detect_darwin():
    with (
        patch("os.path.isdir", return_value=False),
        patch("platform.system", return_value="Darwin"),
    ):
        assert detect_platform() == "darwin"


def test_detect_windows():
    with (
        patch("os.path.isdir", return_value=False),
        patch("platform.system", return_value="Windows"),
    ):
        assert detect_platform() == "windows"


def test_detect_arch_x86_64():
    with patch("platform.machine", return_value="x86_64"):
        assert detect_arch() == "x86_64"


def test_detect_arch_aarch64():
    with patch("platform.machine", return_value="aarch64"):
        assert detect_arch() == "aarch64"


def test_detect_arch_arm64():
    with patch("platform.machine", return_value="arm64"):
        assert detect_arch() == "aarch64"


def test_detect_goarch_amd64():
    with patch("platform.machine", return_value="x86_64"):
        assert detect_goarch() == "amd64"


def test_detect_goarch_arm64():
    with patch("platform.machine", return_value="aarch64"):
        assert detect_goarch() == "arm64"


def test_detect_os_name_linux():
    with patch("dots.platform.detect_platform", return_value="linux"):
        assert detect_os_name() == "linux"


def test_detect_os_name_termux():
    with patch("dots.platform.detect_platform", return_value="termux"):
        assert detect_os_name() == "linux"


def test_detect_os_name_darwin():
    with patch("dots.platform.detect_platform", return_value="darwin"):
        assert detect_os_name() == "darwin"
