"""Integration tests for bootstrapper insertion, update, uninit; marker idempotency."""

from dots.constants import MARKER_END, MARKER_START
from dots.shell import (
    BASH_BOOTSTRAPPER,
    ZSH_BOOTSTRAPPER,
    idempotent_insert,
    remove_marker_block,
)


def test_bootstrapper_into_empty_zshrc(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("")

    idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)

    content = zshrc.read_text()
    assert MARKER_START in content
    assert MARKER_END in content
    assert "source" in content
    assert "[0-9]*.zsh" in content


def test_bootstrapper_updated_existing(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# my config\n" + ZSH_BOOTSTRAPPER + "\n# end\n")

    # Simulate update with slightly different content
    new_bootstrap = ZSH_BOOTSTRAPPER.replace("shell.d", "shell.d.v2")
    idempotent_insert(zshrc, new_bootstrap)

    content = zshrc.read_text()
    assert "shell.d.v2" in content
    assert "# my config" in content
    assert "# end" in content
    assert content.count(MARKER_START) == 1


def test_bootstrapper_idempotent(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("")

    idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)
    content1 = zshrc.read_text()

    changed = idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)
    content2 = zshrc.read_text()

    assert content1 == content2
    assert changed is False


def test_uninit_removes_stanza(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# before\n" + ZSH_BOOTSTRAPPER + "\n# after\n")

    removed = remove_marker_block(zshrc)
    assert removed is True

    content = zshrc.read_text()
    assert MARKER_START not in content
    assert MARKER_END not in content
    assert "# before" in content
    assert "# after" in content


def test_uninit_no_stanza(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# just config\n")

    removed = remove_marker_block(zshrc)
    assert removed is False


def test_bash_bootstrapper_posix(tmp_home):
    assert "[ -d" in BASH_BOOTSTRAPPER
    assert "[ -f" in BASH_BOOTSTRAPPER
    assert '. "$_dots_f"' in BASH_BOOTSTRAPPER


def test_zsh_bootstrapper_zsh_syntax():
    assert "[[ -d" in ZSH_BOOTSTRAPPER
    assert "[[ -f" in ZSH_BOOTSTRAPPER
    assert "source" in ZSH_BOOTSTRAPPER


def test_zsh_sources_sh_and_zsh():
    assert "[0-9]*.sh" in ZSH_BOOTSTRAPPER
    assert "[0-9]*.zsh" in ZSH_BOOTSTRAPPER
    assert ".bash" not in ZSH_BOOTSTRAPPER


def test_bash_sources_sh_and_bash():
    assert "[0-9]*.sh" in BASH_BOOTSTRAPPER
    assert "[0-9]*.bash" in BASH_BOOTSTRAPPER
    assert ".zsh" not in BASH_BOOTSTRAPPER


def test_bootstrapper_into_new_file(tmp_home):
    zshrc = tmp_home / ".zshrc"
    assert not zshrc.exists()

    idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)
    assert zshrc.exists()
    assert MARKER_START in zshrc.read_text()


def test_bootstrapper_creates_parent_dirs(tmp_home):
    # e.g. zshrc = "~/.config/zsh/.zshrc" on a fresh system
    zshrc = tmp_home / ".config" / "zsh" / ".zshrc"
    assert not zshrc.parent.exists()

    idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)
    assert zshrc.exists()
    assert MARKER_START in zshrc.read_text()


def test_bootstrapper_broken_symlink_replaces(tmp_home):
    zshrc = tmp_home / ".zshrc"
    zshrc.symlink_to(tmp_home / "nonexistent-target")
    assert zshrc.is_symlink()
    assert not zshrc.exists()

    idempotent_insert(zshrc, ZSH_BOOTSTRAPPER)
    assert zshrc.exists()
    assert not zshrc.is_symlink()
    assert MARKER_START in zshrc.read_text()
