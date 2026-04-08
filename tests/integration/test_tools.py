"""Integration tests for install dispatch, method fallback, github mock."""

import tarfile
import zipfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# ── Security: archive extraction ────────────────────────────────────────────


def test_tar_path_traversal_rejected(dots, tmp_path):
    """Tar archive with path traversal entry is rejected."""
    from dots.tools import ToolInstallError, _safe_tar_extractall

    # Create a malicious tar with a ../../ path
    tar_path = tmp_path / "evil.tar.gz"
    extract_dir = tmp_path / "extracted"
    extract_dir.mkdir()

    with tarfile.open(str(tar_path), "w:gz") as tf:
        import io

        data = b"pwned"
        info = tarfile.TarInfo(name="../../etc/evil")
        info.size = len(data)
        tf.addfile(info, io.BytesIO(data))

    with tarfile.open(str(tar_path), "r:gz") as tf:
        with pytest.raises(ToolInstallError, match="path escapes"):
            _safe_tar_extractall(tf, extract_dir)


def test_tar_absolute_symlink_rejected(dots, tmp_path):
    """Tar archive with absolute symlink target is rejected."""
    from dots.tools import ToolInstallError, _safe_tar_extractall

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


def test_zip_path_traversal_rejected(dots, tmp_path):
    """Zip archive with path traversal entry is rejected."""
    from dots.tools import ToolInstallError, _safe_zip_extractall

    zip_path = tmp_path / "evil.zip"
    extract_dir = tmp_path / "extracted"
    extract_dir.mkdir()

    with zipfile.ZipFile(str(zip_path), "w") as zf:
        zf.writestr("../../etc/evil", "pwned")

    with zipfile.ZipFile(str(zip_path), "r") as zf:
        with pytest.raises(ToolInstallError, match="path escapes"):
            _safe_zip_extractall(zf, extract_dir)


def test_safe_tar_extraction_works(dots, tmp_path):
    """Normal tar extraction still works."""
    from dots.tools import _safe_tar_extractall

    tar_path = tmp_path / "good.tar.gz"
    extract_dir = tmp_path / "extracted"

    import io

    with tarfile.open(str(tar_path), "w:gz") as tf:
        data = b"hello"
        info = tarfile.TarInfo(name="mybin")
        info.size = len(data)
        tf.addfile(info, io.BytesIO(data))

    with tarfile.open(str(tar_path), "r:gz") as tf:
        _safe_tar_extractall(tf, extract_dir)

    assert (extract_dir / "mybin").exists()
    assert (extract_dir / "mybin").read_bytes() == b"hello"


def test_method_fallback_order(dots):
    """First matching method (by platform + manager) used."""
    tool = dots.Tool(name="rg", check="rg --version")
    tool.install = [
        dots.ToolInstall(method="pkg", package="ripgrep", only=["termux"]),
        dots.ToolInstall(method="apt", package="ripgrep", only=["linux"]),
        dots.ToolInstall(method="cargo", package="ripgrep"),
    ]

    def _which(x):
        return "/usr/bin/" + x if x in ("apt-get", "cargo") else None

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", side_effect=_which),
    ):
        inst = dots.find_install_method(tool)

    assert inst is not None
    assert inst.method == "apt"


def test_platform_filter_skips(dots):
    """Method with non-matching platform skipped."""
    tool = dots.Tool(name="rg", check="rg --version")
    tool.install = [
        dots.ToolInstall(method="pkg", package="ripgrep", only=["termux"]),
    ]

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", return_value="/usr/bin/pkg"),
    ):
        inst = dots.find_install_method(tool)

    assert inst is None


def test_unavailable_manager_skipped(dots):
    """Method with unavailable manager skipped."""
    tool = dots.Tool(name="rg", check="rg --version")
    tool.install = [
        dots.ToolInstall(method="brew", package="ripgrep"),
        dots.ToolInstall(method="cargo", package="ripgrep"),
    ]

    def mock_which(name):
        if name == "cargo":
            return "/usr/bin/cargo"
        return None

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("shutil.which", side_effect=mock_which),
    ):
        inst = dots.find_install_method(tool)

    assert inst is not None
    assert inst.method == "cargo"


def test_tool_is_installed_check(dots):
    """tool_is_installed uses check command."""
    tool = dots.Tool(name="rg", check="rg --version")

    with patch("subprocess.run") as mock_run:
        mock_run.return_value = MagicMock(returncode=0)
        assert dots.tool_is_installed(tool) is True

        mock_run.return_value = MagicMock(returncode=1)
        assert dots.tool_is_installed(tool) is False


def test_install_apt(dots):
    """apt install calls apt-get."""
    tool = dots.Tool(name="rg")
    inst = dots.ToolInstall(method="apt", package="ripgrep")

    with (
        patch("dots.platform.detect_platform", return_value="linux"),
        patch("os.getuid", return_value=1000),
        patch("dots.utils.run") as mock_run,
    ):
        result = dots.install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "apt"
    cmd = mock_run.call_args[0][0]
    assert "sudo" in cmd
    assert "apt-get" in cmd


def test_install_apt_termux_error(dots):
    """apt on Termux raises error."""
    tool = dots.Tool(name="rg")
    inst = dots.ToolInstall(method="apt", package="ripgrep")

    with patch("dots.platform.detect_platform", return_value="termux"):
        with pytest.raises(dots.ToolInstallError, match="Termux"):
            dots.install_tool(tool, inst, Path("/tmp/bin"))


def test_install_brew(dots):
    """brew install calls brew."""
    tool = dots.Tool(name="rg")
    inst = dots.ToolInstall(method="brew", package="ripgrep")

    with patch("dots.utils.run") as mock_run:
        result = dots.install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "brew"
    cmd = mock_run.call_args[0][0]
    assert "brew" in cmd


def test_install_cargo(dots):
    """cargo install calls cargo."""
    tool = dots.Tool(name="rg")
    inst = dots.ToolInstall(method="cargo", package="ripgrep", binary="rg")

    with patch("dots.utils.run") as mock_run:
        result = dots.install_tool(tool, inst, Path("/tmp/bin"))

    assert result == "cargo"
    cmd = mock_run.call_args[0][0]
    assert "cargo" in cmd
    assert "--bin" in cmd


def test_install_manual(dots, capsys):
    """manual method prints note."""
    tool = dots.Tool(name="thing")
    inst = dots.ToolInstall(method="manual", note="Install from website")

    result = dots.install_tool(tool, inst, Path("/tmp/bin"))
    assert result == "manual"
    assert "Install from website" in capsys.readouterr().out


def test_install_unknown_method(dots):
    """Unknown method raises error."""
    tool = dots.Tool(name="thing")
    inst = dots.ToolInstall(method="flatpak", package="thing")

    with pytest.raises(dots.ToolInstallError, match="Unknown"):
        dots.install_tool(tool, inst, Path("/tmp/bin"))


def test_glob_match(dots):
    """Asset glob pattern matching."""
    from dots.tools import _glob_match

    assert _glob_match("ripgrep-*.tar.gz", "ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz")
    assert not _glob_match("ripgrep-*.zip", "ripgrep-14.1.0.tar.gz")
    assert _glob_match("bat-v*-aarch64-*", "bat-v0.24.0-aarch64-unknown-linux-musl.tar.gz")


def test_github_rate_limit_error(dots):
    """GitHub rate limit gives helpful error."""
    from email.message import Message
    from io import BytesIO
    from urllib.error import HTTPError

    headers = Message()
    headers["X-RateLimit-Reset"] = "9999999999"

    err = HTTPError("url", 403, "Forbidden", headers, BytesIO(b""))

    with patch("dots.tools.urlopen", side_effect=err):
        with pytest.raises(dots.ToolInstallError, match="rate limit"):
            dots.github_get_latest_release("owner/repo")
