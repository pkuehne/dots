"""Integration tests for bootstrapper insertion, update, uninit; marker idempotency."""


def test_bootstrapper_into_empty_zshrc(dots, tmp_home):
    """Bootstrapper inserted into empty .zshrc."""
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("")

    dots.idempotent_insert(zshrc, dots.ZSH_BOOTSTRAPPER)

    content = zshrc.read_text()
    assert dots.MARKER_START in content
    assert dots.MARKER_END in content
    assert "source" in content
    assert "[0-9]*.zsh" in content


def test_bootstrapper_updated_existing(dots, tmp_home):
    """Bootstrapper updated in existing .zshrc (marker replacement)."""
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# my config\n" + dots.ZSH_BOOTSTRAPPER + "\n# end\n")

    # Simulate update with slightly different content
    new_bootstrap = dots.ZSH_BOOTSTRAPPER.replace("shell.d", "shell.d.v2")
    dots.idempotent_insert(zshrc, new_bootstrap)

    content = zshrc.read_text()
    assert "shell.d.v2" in content
    assert "# my config" in content
    assert "# end" in content
    assert content.count(dots.MARKER_START) == 1


def test_bootstrapper_idempotent(dots, tmp_home):
    """Bootstrapper does not duplicate if already present."""
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("")

    dots.idempotent_insert(zshrc, dots.ZSH_BOOTSTRAPPER)
    content1 = zshrc.read_text()

    changed = dots.idempotent_insert(zshrc, dots.ZSH_BOOTSTRAPPER)
    content2 = zshrc.read_text()

    assert content1 == content2
    assert changed is False


def test_uninit_removes_stanza(dots, tmp_home):
    """uninit removes stanza, leaves surrounding content intact."""
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# before\n" + dots.ZSH_BOOTSTRAPPER + "\n# after\n")

    removed = dots.remove_marker_block(zshrc)
    assert removed is True

    content = zshrc.read_text()
    assert dots.MARKER_START not in content
    assert dots.MARKER_END not in content
    assert "# before" in content
    assert "# after" in content


def test_uninit_no_stanza(dots, tmp_home):
    """uninit on file without stanza returns False."""
    zshrc = tmp_home / ".zshrc"
    zshrc.write_text("# just config\n")

    removed = dots.remove_marker_block(zshrc)
    assert removed is False


def test_bash_bootstrapper_posix(dots, tmp_home):
    """Bash bootstrapper uses [ ] not [[ ]] and . not source."""
    assert "[ -d" in dots.BASH_BOOTSTRAPPER
    assert "[ -f" in dots.BASH_BOOTSTRAPPER
    assert '. "$_dots_f"' in dots.BASH_BOOTSTRAPPER


def test_zsh_bootstrapper_zsh_syntax(dots):
    """Zsh bootstrapper uses [[ ]] and source."""
    assert "[[ -d" in dots.ZSH_BOOTSTRAPPER
    assert "[[ -f" in dots.ZSH_BOOTSTRAPPER
    assert "source" in dots.ZSH_BOOTSTRAPPER


def test_zsh_sources_sh_and_zsh(dots):
    """Zsh bootstrapper sources .sh and .zsh files."""
    assert "[0-9]*.sh" in dots.ZSH_BOOTSTRAPPER
    assert "[0-9]*.zsh" in dots.ZSH_BOOTSTRAPPER
    assert ".bash" not in dots.ZSH_BOOTSTRAPPER


def test_bash_sources_sh_and_bash(dots):
    """Bash bootstrapper sources .sh and .bash files."""
    assert "[0-9]*.sh" in dots.BASH_BOOTSTRAPPER
    assert "[0-9]*.bash" in dots.BASH_BOOTSTRAPPER
    assert ".zsh" not in dots.BASH_BOOTSTRAPPER


def test_bootstrapper_into_new_file(dots, tmp_home):
    """Bootstrapper creates file if it doesn't exist."""
    zshrc = tmp_home / ".zshrc"
    assert not zshrc.exists()

    dots.idempotent_insert(zshrc, dots.ZSH_BOOTSTRAPPER)
    assert zshrc.exists()
    assert dots.MARKER_START in zshrc.read_text()
