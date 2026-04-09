"""Integration tests for managed.gitconfig write + [include] insertion."""

from dots.config import Config
from dots.constants import MARKER_START
from dots.git import GIT_INCLUDE_BLOCK, generate_gitconfig
from dots.shell import idempotent_insert, remove_marker_block


def test_git_init_generates_config(tmp_home):
    config = Config()
    config.git.name = "Test User"
    config.git.email = "test@example.com"
    config.git.editor = "nvim"
    config.tools = []

    git_dir = tmp_home / ".config" / "dots" / "git"
    git_dir.mkdir(parents=True)

    content = generate_gitconfig(config)
    (git_dir / "managed.gitconfig").write_text(content)

    result = (git_dir / "managed.gitconfig").read_text()
    assert "name = Test User" in result
    assert "email = test@example.com" in result


def test_include_inserted_into_gitconfig(tmp_home):
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text("[user]\n    name = Existing\n")

    idempotent_insert(gitconfig, GIT_INCLUDE_BLOCK)

    content = gitconfig.read_text()
    assert "[include]" in content
    assert "managed.gitconfig" in content
    assert "[user]" in content  # Original content preserved


def test_include_not_duplicated(tmp_home):
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text("[user]\n    name = Test\n")

    idempotent_insert(gitconfig, GIT_INCLUDE_BLOCK)
    idempotent_insert(gitconfig, GIT_INCLUDE_BLOCK)

    content = gitconfig.read_text()
    assert content.count("[include]") == 1


def test_uninit_removes_include(tmp_home):
    gitconfig = tmp_home / ".gitconfig"
    gitconfig.write_text(
        "[user]\n    name = Test\n" + GIT_INCLUDE_BLOCK + "\n[core]\n    editor = vim\n"
    )

    remove_marker_block(gitconfig)

    content = gitconfig.read_text()
    assert MARKER_START not in content
    assert "[user]" in content
    assert "[core]" in content
