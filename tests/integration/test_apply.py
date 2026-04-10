"""Integration tests for full deploy cycle on tmp dirs; idempotency."""

import os
import stat
from pathlib import Path
from unittest.mock import patch

from dots.commands import cmd_apply
from dots.config import load_config

# ── Security: path validation ───────────────────────────────────────────────


def test_src_outside_repo_rejected(tmp_repo, tmp_home):
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "../../../etc/passwd"
dst = "~/.stolen"
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    assert not (tmp_home / ".stolen").exists()


def test_dst_outside_home_rejected(tmp_repo, tmp_home):
    (tmp_repo / "files" / "evil").write_text("pwned")
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "files/evil"
dst = "/tmp/pwned"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    assert not Path("/tmp/pwned").exists()


def test_secret_written_with_restricted_mode(tmp_repo, tmp_home):
    # We can't easily test the race-free write directly, but we can verify
    # the final permissions on a secret file deployed in copy mode
    (tmp_repo / "files" / "token").write_text("secret-value")
    (tmp_repo / "dots.toml").write_text("""\
[[file]]
src = "files/token"
dst = "~/.token"
mode = "600"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    target = tmp_home / ".token"
    assert target.exists()
    assert stat.S_IMODE(target.stat().st_mode) == 0o600


def test_symlink_created(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    target = tmp_home / ".vimrc"
    assert target.is_symlink()
    assert target.resolve() == (tmp_repo / "files" / ".vimrc").resolve()


def test_symlink_idempotent(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)
        result1 = os.lstat(str(tmp_home / ".vimrc")).st_ino
        cmd_apply(config)
        result2 = os.lstat(str(tmp_home / ".vimrc")).st_ino

    assert result1 == result2  # Same inode, not recreated


def test_stale_symlink_replaced(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # Create stale symlink
    target = tmp_home / ".vimrc"
    target.symlink_to("/nonexistent/old/path")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    assert target.is_symlink()
    assert target.resolve() == (tmp_repo / "files" / ".vimrc").resolve()
    # Backup should exist
    assert (tmp_home / ".vimrc.dots-bak").exists() or (tmp_home / ".vimrc.dots-bak").is_symlink()


def test_existing_file_backed_up(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    # Create existing file
    target = tmp_home / ".vimrc"
    target.write_text("old content")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    assert target.is_symlink()
    bak = tmp_home / ".vimrc.dots-bak"
    assert bak.exists()
    assert bak.read_text() == "old content"


def test_ssh_dir_700(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".ssh").mkdir(parents=True)
    (tmp_repo / "files" / ".ssh" / "config").write_text("# ssh config")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    ssh_dir = tmp_home / ".ssh"
    assert ssh_dir.exists()
    assert stat.S_IMODE(ssh_dir.stat().st_mode) == 0o700


def test_copy_mode(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config, force_copy=True)

    target = tmp_home / ".vimrc"
    assert not target.is_symlink()
    assert target.read_text() == "set nocompatible"


def test_dry_run_no_side_effects(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config, dry_run=True)

    assert not (tmp_home / ".vimrc").exists()


def test_mode_applied(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".netrc").write_text("machine example.com")
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[[file]]
src = "files/.netrc"
dst = "~/.netrc"
mode = "600"
link = false
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(toml, tmp_repo)
        cmd_apply(config)

    target = tmp_home / ".netrc"
    assert target.exists()
    assert stat.S_IMODE(target.stat().st_mode) == 0o600


def test_nested_dirs_created(tmp_repo, tmp_home):
    (tmp_repo / "files" / ".config" / "nvim").mkdir(parents=True)
    (tmp_repo / "files" / ".config" / "nvim" / "init.lua").write_text("-- nvim")
    (tmp_repo / "dots.toml").write_text("[meta]\nversion = 1\n")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    assert (tmp_home / ".config" / "nvim" / "init.lua").is_symlink()


# ── Shell managed via apply ──────────────────────────────────────────────────


def test_apply_installs_bootstrapper(tmp_repo, tmp_home):
    """dots apply with shell.managed should install bootstrapper in rc files."""
    (tmp_repo / "dots.toml").write_text("""\
[meta]
version = 1

[shell]
managed = true
""")
    zshrc = tmp_home / ".zshrc"
    bashrc = tmp_home / ".bashrc"

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    from dots.constants import MARKER_START

    assert zshrc.exists()
    assert MARKER_START in zshrc.read_text()
    assert bashrc.exists()
    assert MARKER_START in bashrc.read_text()


def test_apply_bootstrapper_idempotent(tmp_repo, tmp_home):
    """Running apply twice should not duplicate the bootstrapper block."""
    (tmp_repo / "dots.toml").write_text("""\
[meta]
version = 1

[shell]
managed = true
""")

    from dots.constants import MARKER_START

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)
        cmd_apply(config)

    zshrc_text = (tmp_home / ".zshrc").read_text()
    assert zshrc_text.count(MARKER_START) == 1


def test_apply_generates_shell_snippets(tmp_repo, tmp_home):
    """dots apply with shell.managed should generate snippet files."""
    (tmp_repo / "dots.toml").write_text("""\
[meta]
version = 1

[shell]
managed = true
path = ["~/.local/bin"]

[env]
EDITOR = "vim"
""")

    with patch("dots.platform.detect_platform", return_value="linux"):
        config = load_config(tmp_repo / "dots.toml", tmp_repo)
        cmd_apply(config)

    shell_dir = tmp_home / ".config" / "dots" / "shell.d"
    assert (shell_dir / "010-env.sh").exists()
    assert (shell_dir / "020-path.sh").exists()
    assert "EDITOR" in (shell_dir / "010-env.sh").read_text()
