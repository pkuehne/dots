"""Integration tests for clone, update, status (git mocked)."""

from unittest.mock import patch

import pytest

from dots.config import RepoEntry
from dots.errors import DotsError
from dots.repos import clone_repo, update_repo


def test_clone_new_repo(tmp_home):
    repo = RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        shallow=True,
    )
    with patch("dots.utils.run") as mock_run:
        result = clone_repo(repo)

    assert result == "ok"
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "git" in cmd
    assert "clone" in cmd
    assert "--depth" in cmd


def test_clone_already_exists(tmp_home):
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = RepoEntry(name="test", repo="user/test", dst=str(dst))
    result = clone_repo(repo)
    assert result == "already"


def test_clone_dir_exists_not_git(tmp_home):
    dst = tmp_home / "test-repo"
    dst.mkdir()

    repo = RepoEntry(name="test", repo="user/test", dst=str(dst))
    with pytest.raises(DotsError, match="not a git repository|Cannot clone"):
        clone_repo(repo)


def test_clone_with_ref(tmp_home):
    repo = RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        ref="v1.0",
    )
    with patch("dots.utils.run") as mock_run:
        clone_repo(repo)

    cmd = mock_run.call_args[0][0]
    assert "--branch" in cmd
    assert "v1.0" in cmd


def test_clone_on_install_hook(tmp_home):
    repo = RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        on_install="echo installed",
    )
    with patch("dots.utils.run") as mock_run:
        clone_repo(repo)

    # Should be called twice: git clone + on_install
    assert mock_run.call_count == 2
    second_call = mock_run.call_args_list[1]
    assert second_call[0][0] == "echo installed"


def test_update_repo(tmp_home):
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = RepoEntry(name="test", repo="user/test", dst=str(dst))
    with patch("dots.utils.run") as mock_run:
        result = update_repo(repo)

    assert result == "ok"
    cmd = mock_run.call_args[0][0]
    assert "pull" in cmd


def test_update_shallow(tmp_home):
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = RepoEntry(name="test", repo="user/test", dst=str(dst), shallow=True)
    with patch("dots.utils.run") as mock_run:
        update_repo(repo)

    calls = [c[0][0] for c in mock_run.call_args_list]
    assert any("fetch" in c for c in calls)
    assert any("reset" in c for c in calls)


def test_update_missing_repo(tmp_home):
    repo = RepoEntry(name="test", repo="user/test", dst=str(tmp_home / "nope"))
    result = update_repo(repo)
    assert result == "missing"
