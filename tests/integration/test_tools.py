"""Integration tests for install dispatch, method fallback, github mock."""

import io
import tarfile
import zipfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from dots.config import Tool, ToolInstall
from dots.errors import ToolInstallError
from dots.tools import (
    _glob_match,
    _safe_tar_extractall,
    _safe_zip_extractall,
    find_install_method,
    github_get_latest_release,
    install_tool,
    tool_is_installed,
)

# ── Security: archive extraction ────────────────────────────────────────────


def test_tar_path_traversal_rejected(tmp_path):
    tar_path = tmp_path / "evil.tar.gz"
    extract_dir = tmp_path / "extracted"
    extract_dir.mkdir()

    with tarfile.open(str(tar_path), "w:gz") as tf:
        data = b"pwned"
        info = tarfile.TarInfo(name="../../etc/evil")
        info.size = len(data)
        tf.addfile(info, io.BytesIO(data))

    with tarfile.open(str(tar_path), "r:gz") as tf:
        with pytest.raises(ToolInstallError, match="path escapes"):
            _safe_tar_extractall(tf, extract_dir)


def test_tar_absolute_symlink_rejected(tmp_path):
    tar_path = tmp_path / "evil.tar.gz"
    extract_dir = tmp_path / "extracted"
    extract_dir.mkdir()

    with tarfile.open(str(tar_path), "w:gz") as tf:
        info = tarfile.TarInfo(name="link")
        info.type = tarfile.SYMTYPE
        info.linkname = "/etc/passwd"
        tf.addfile(info)

    with tarfile.open(str(tar_path), "r:gz") as tf:
        with pytest.raises(ToolInstallError, match="absolute target"):
            _safe_tar_extractall(tf, extract_dir)


def test_zip_path_traversal_rejected(tmp_path):
    zip_path = tmp_path / "evil.zip"
    extract_dir = tmp_path / "extracted"
    extract_dir.mkdir()

    with zipfile.ZipFile(str(zip_path), "w") as zf:
        zf.writestr("../../etc/evil", "pwned")

    with zipfile.ZipFile(str(zip_path), "r") as zf:
        with pytest.raises(ToolInstallError, match="path escapes"):
            _safe_zip_extractall(zf, extract_dir)


def test_safe_tar_extraction_works(tmp_path):
    tar_path = tmp_path / "good.tar.gz"
    extract_dir = tmp_path / "extracted"

    with tarfile.open(str(tar_path), "w:gz") as tf:
        data = b"hello"
        info = tarfile.TarInfo(name="mybin")
        info.size = len(data)
        tf.addfile(info, io.BytesIO(data))

    with tarfile.open(str(tar_path), "r:gz") as tf:
        _safe_tar_extractall(tf, extract_dir)

    assert (extract_dir / "mybin").exists()
    assert (extract_dir / "mybin").read_bytes() == b"hello"


# ── Install method selection ────────────────────────────────────────────────


def test_method_fallback_order():
    tool = Tool(name="rg", check="rg --version")
    tool.install = [
        ToolInstall(method="pkg", package="ripgrep", only=["termux"]),
        ToolInstall(method="apt", package="ripgrep", only=["linux"]),
        ToolInstall(method="cargo", package="ripgrep"),
    ]

    def _which(x):
        return "/usr/bin/" + x if x in ("apt-get", "cargo") else None

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", side_effect=_which),
    ):
        inst = find_install_method(tool)

    assert inst is not None
    assert inst.method == "apt"


def test_platform_filter_skips():
    tool = Tool(name="rg", check="rg --version")
    tool.install = [
        ToolInstall(method="pkg", package="ripgrep", only=["termux"]),
    ]

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", return_value="/usr/bin/pkg"),
    ):
        inst = find_install_method(tool)

    assert inst is None


def test_unavailable_manager_skipped():
    tool = Tool(name="rg", check="rg --version")
    tool.install = [
        ToolInstall(method="brew", package="ripgrep"),
        ToolInstall(method="cargo", package="ripgrep"),
    ]

    def mock_which(name):
        if name == "cargo":
            return "/usr/bin/cargo"
        return None

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", side_effect=mock_which),
    ):
        inst = find_install_method(tool)

    assert inst is not None
    assert inst.method == "cargo"


def test_tool_is_installed_check():
    tool = Tool(name="rg", check="rg --version")

    with patch("subprocess.run") as mock_run:
        mock_run.return_value = MagicMock(returncode=0)
        assert tool_is_installed(tool) is True

        mock_run.return_value = MagicMock(returncode=1)
        assert tool_is_installed(tool) is False


# ── Install dispatch ────────────────────────────────────────────────────────


def test_install_apt():
    tool = Tool(name="rg")
    inst = ToolInstall(method="apt", package="ripgrep")

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("os.getuid", return_value=1000),
        patch("dots.utils.run") as mock_run,
    ):
        result = install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "apt"
    cmd = mock_run.call_args[0][0]
    assert "sudo" in cmd
    assert "apt-get" in cmd


def test_install_apt_termux_error():
    tool = Tool(name="rg")
    inst = ToolInstall(method="apt", package="ripgrep")

    with patch("dots.platform.detect_platform", return_value="termux"):
        with pytest.raises(ToolInstallError, match="Termux"):
            install_tool(tool, inst, Path("/tmp/bin"))


def test_install_brew():
    tool = Tool(name="rg")
    inst = ToolInstall(method="brew", package="ripgrep")

    with patch("dots.utils.run") as mock_run:
        result = install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "brew"
    cmd = mock_run.call_args[0][0]
    assert "brew" in cmd


def test_install_cargo():
    tool = Tool(name="rg")
    inst = ToolInstall(method="cargo", package="ripgrep", binary="rg")

    with patch("dots.utils.run") as mock_run:
        result = install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "cargo"
    cmd = mock_run.call_args[0][0]
    assert "cargo" in cmd
    assert "--bin" in cmd


def test_install_manual(capsys):
    tool = Tool(name="thing")
    inst = ToolInstall(method="manual", note="Install from website")

    result = install_tool(tool, inst, Path("/tmp/bin"))
    assert result == "manual"
    assert "Install from website" in capsys.readouterr().out


def test_install_unknown_method():
    tool = Tool(name="thing")
    inst = ToolInstall(method="flatpak", package="thing")

    with pytest.raises(ToolInstallError, match="Unknown"):
        install_tool(tool, inst, Path("/tmp/bin"))


def test_glob_match():
    assert _glob_match("ripgrep-*.tar.gz", "ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz")
    assert not _glob_match("ripgrep-*.zip", "ripgrep-14.1.0.tar.gz")
    assert _glob_match("bat-v*-aarch64-*", "bat-v0.24.0-aarch64-unknown-linux-musl.tar.gz")


def test_github_rate_limit_error():
    from email.message import Message
    from urllib.error import HTTPError

    headers = Message()
    headers["X-RateLimit-Reset"] = "9999999999"

    err = HTTPError("url", 403, "Forbidden", headers, io.BytesIO(b""))

    with patch("dots.tools.urlopen", side_effect=err):
        with pytest.raises(ToolInstallError, match="rate limit"):
            github_get_latest_release("owner/repo")
