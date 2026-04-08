"""Shared fixtures for dots test suite."""

import os
import shutil
import sys
import tempfile
from pathlib import Path
from unittest.mock import patch

import pytest

# Add repo root to path so we can import dots
REPO_ROOT = Path(__file__).parent.parent
sys.path.insert(0, str(REPO_ROOT))

# Import dots as a module (no .py extension)
import importlib.util
import importlib.machinery

_dots_path = REPO_ROOT / "dots"
_loader = importlib.machinery.SourceFileLoader("dots_mod", str(_dots_path))
_spec = importlib.util.spec_from_loader("dots_mod", _loader, origin=str(_dots_path))
dots_mod = importlib.util.module_from_spec(_spec)
sys.modules["dots_mod"] = dots_mod  # Required for @dataclass to resolve module
_loader.exec_module(dots_mod)


@pytest.fixture
def dots():
    """Return the dots module."""
    return dots_mod


@pytest.fixture
def tmp_home(tmp_path):
    """Provide a temporary home directory."""
    home = tmp_path / "home"
    home.mkdir()
    with patch.dict(os.environ, {"HOME": str(home)}), \
         patch("pathlib.Path.home", return_value=home):
        yield home


@pytest.fixture
def tmp_repo(tmp_path):
    """Provide a temporary dotfiles repo directory."""
    repo = tmp_path / "dotfiles"
    repo.mkdir()
    (repo / "files").mkdir()
    (repo / "files.d").mkdir()
    (repo / "shell").mkdir()
    return repo


@pytest.fixture
def minimal_repo(tmp_path):
    """Copy the minimal fixture to a temp dir."""
    src = REPO_ROOT / "tests" / "fixtures" / "minimal"
    dst = tmp_path / "minimal"
    shutil.copytree(str(src), str(dst))
    return dst


@pytest.fixture
def full_repo(tmp_path):
    """Copy the full fixture to a temp dir."""
    src = REPO_ROOT / "tests" / "fixtures" / "full"
    dst = tmp_path / "full"
    shutil.copytree(str(src), str(dst))
    return dst


@pytest.fixture
def mock_platform(dots):
    """Context manager to mock platform detection."""
    def _mock(platform_name):
        return patch.object(dots_mod, "detect_platform", return_value=platform_name)
    return _mock


@pytest.fixture
def mock_hostname(dots):
    """Context manager to mock hostname."""
    def _mock(hostname):
        return patch.object(dots_mod, "get_hostname", return_value=hostname)
    return _mock
