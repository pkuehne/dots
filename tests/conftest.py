import os
import shutil
import sys
from pathlib import Path
from unittest.mock import patch

import pytest

# Add src/ to path so we can import dots package
REPO_ROOT = Path(__file__).parent.parent
sys.path.insert(0, str(REPO_ROOT / "src"))


@pytest.fixture
def tmp_home(tmp_path):
    home = tmp_path / "home"
    home.mkdir()
    with patch.dict(os.environ, {"HOME": str(home)}), patch("pathlib.Path.home", return_value=home):
        yield home


@pytest.fixture
def tmp_repo(tmp_path):
    repo = tmp_path / "dotfiles"
    repo.mkdir()
    (repo / "files").mkdir()
    (repo / "files.d").mkdir()
    (repo / "shell").mkdir()
    return repo


@pytest.fixture
def minimal_repo(tmp_path):
    src = REPO_ROOT / "tests" / "fixtures" / "minimal"
    dst = tmp_path / "minimal"
    shutil.copytree(str(src), str(dst))
    return dst


@pytest.fixture
def full_repo(tmp_path):
    src = REPO_ROOT / "tests" / "fixtures" / "full"
    dst = tmp_path / "full"
    shutil.copytree(str(src), str(dst))
    return dst


@pytest.fixture
def mock_platform():
    def _mock(platform_name):
        return patch("dots.platform.detect_platform", return_value=platform_name)

    return _mock


@pytest.fixture
def mock_hostname():
    def _mock(hostname):
        return patch("dots.platform.get_hostname", return_value=hostname)

    return _mock
