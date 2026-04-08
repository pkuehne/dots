"""Integration tests for clone, update, status (git mocked)."""

import os
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest


def test_clone_new_repo(dots, tmp_home):
    """Clone creates directory and runs git clone."""
    repo = dots.RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        shallow=True,
    )
    with patch.object(dots, "run") as mock_run:
        result = dots.clone_repo(repo)

    assert result == "ok"
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "git" in cmd
    assert "clone" in cmd
    assert "--depth" in cmd


def test_clone_already_exists(dots, tmp_home):
    """Already-cloned repos return 'already'."""
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = dots.RepoEntry(name="test", repo="user/test", dst=str(dst))
    result = dots.clone_repo(repo)
    assert result == "already"


def test_clone_dir_exists_not_git(dots, tmp_home):
    """Existing non-git directory raises error."""
    dst = tmp_home / "test-repo"
    dst.mkdir()

    repo = dots.RepoEntry(name="test", repo="user/test", dst=str(dst))
    with pytest.raises(dots.DotsError, match="not a git repository|Cannot clone"):
        dots.clone_repo(repo)


def test_clone_with_ref(dots, tmp_home):
    """Clone with ref passes --branch."""
    repo = dots.RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        ref="v1.0",
    )
    with patch.object(dots, "run") as mock_run:
        dots.clone_repo(repo)

    cmd = mock_run.call_args[0][0]
    assert "--branch" in cmd
    assert "v1.0" in cmd


def test_clone_on_install_hook(dots, tmp_home):
    """on_install command runs after clone."""
    repo = dots.RepoEntry(
        name="test",
        repo="user/test",
        dst=str(tmp_home / "test-repo"),
        on_install="echo installed",
    )
    with patch.object(dots, "run") as mock_run:
        dots.clone_repo(repo)

    # Should be called twice: git clone + on_install
    assert mock_run.call_count == 2
    second_call = mock_run.call_args_list[1]
    assert second_call[0][0] == "echo installed"


def test_update_repo(dots, tmp_home):
    """Update runs git pull."""
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = dots.RepoEntry(name="test", repo="user/test", dst=str(dst))
    with patch.object(dots, "run") as mock_run:
        result = dots.update_repo(repo)

    assert result == "ok"
    cmd = mock_run.call_args[0][0]
    assert "pull" in cmd


def test_update_shallow(dots, tmp_home):
    """Shallow update uses fetch + reset."""
    dst = tmp_home / "test-repo"
    dst.mkdir()
    (dst / ".git").mkdir()

    repo = dots.RepoEntry(name="test", repo="user/test", dst=str(dst), shallow=True)
    with patch.object(dots, "run") as mock_run:
        dots.update_repo(repo)

    calls = [c[0][0] for c in mock_run.call_args_list]
    assert any("fetch" in c for c in calls)
    assert any("reset" in c for c in calls)


def test_update_missing_repo(dots, tmp_home):
    """Update on missing repo returns 'missing'."""
    repo = dots.RepoEntry(name="test", repo="user/test", dst=str(tmp_home / "nope"))
    result = dots.update_repo(repo)
    assert result == "missing"
