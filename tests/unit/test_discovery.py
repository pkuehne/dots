"""Tests for file discovery from files/ and files.d/."""


def test_files_dir_maps_to_home(dots, tmp_repo, tmp_home):
    """files/ with nested directories maps correctly to ~."""
    (tmp_repo / "files" / ".config" / "nvim").mkdir(parents=True)
    (tmp_repo / "files" / ".config" / "nvim" / "init.lua").write_text("-- nvim")
    (tmp_repo / "files" / ".gitconfig").write_text("[user]")

    discovered = dots.discover_files(tmp_repo, "linux")
    assert len(discovered) == 2
    srcs = {e.src for e in discovered}
    assert "files/.gitconfig" in srcs
    assert "files/.config/nvim/init.lua" in srcs


def test_files_d_platform_only(dots, tmp_repo, tmp_home):
    """files.d/termux/ only discovered on termux platform."""
    (tmp_repo / "files.d" / "termux").mkdir(parents=True)
    (tmp_repo / "files.d" / "termux" / "justfile").write_text("default:")

    termux = dots.discover_files(tmp_repo, "termux")
    linux = dots.discover_files(tmp_repo, "linux")

    assert len(termux) == 1
    assert termux[0].only == ["termux"]
    assert len(linux) == 0


def test_files_d_linux_not_on_termux(dots, tmp_repo, tmp_home):
    """files.d/linux/ not deployed on termux."""
    (tmp_repo / "files.d" / "linux").mkdir(parents=True)
    (tmp_repo / "files.d" / "linux" / ".config" / "systemd").mkdir(parents=True)
    (tmp_repo / "files.d" / "linux" / ".config" / "systemd" / "user.conf").write_text("x")

    termux = dots.discover_files(tmp_repo, "termux")
    linux = dots.discover_files(tmp_repo, "linux")

    assert len(termux) == 0
    assert len(linux) == 1


def test_j2_detected_as_template(dots, tmp_repo, tmp_home):
    """.j2 files detected as templates, not symlinked."""
    (tmp_repo / "files").mkdir(exist_ok=True)
    (tmp_repo / "files" / "aliases.sh.j2").write_text("# {{ name }}")

    discovered = dots.discover_files(tmp_repo, "linux")
    assert len(discovered) == 1
    assert discovered[0].template is True
    assert not discovered[0].dst.endswith(".j2")


def test_age_detected_as_secret(dots, tmp_repo, tmp_home):
    """.age files detected as secrets, not symlinked."""
    (tmp_repo / "files" / ".ssh").mkdir(parents=True)
    (tmp_repo / "files" / ".ssh" / "id_ed25519.age").write_text("encrypted")

    discovered = dots.discover_files(tmp_repo, "linux")
    assert len(discovered) == 1
    assert discovered[0].secret is True
    assert not discovered[0].dst.endswith(".age")
    assert discovered[0].dst.endswith("id_ed25519")


def test_git_dir_skipped(dots, tmp_repo, tmp_home):
    """.git/ inside files/ is skipped."""
    (tmp_repo / "files" / ".git").mkdir(parents=True)
    (tmp_repo / "files" / ".git" / "config").write_text("x")
    (tmp_repo / "files" / ".gitconfig").write_text("y")

    discovered = dots.discover_files(tmp_repo, "linux")
    srcs = {e.src for e in discovered}
    assert "files/.gitconfig" in srcs
    assert not any(s.endswith(".git/config") or "/.git/" in s for s in srcs)


def test_swap_files_skipped(dots, tmp_repo, tmp_home):
    """Swap files and tilde backups skipped."""
    (tmp_repo / "files" / ".vimrc.swp").write_text("")
    (tmp_repo / "files" / ".vimrc~").write_text("")
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")

    discovered = dots.discover_files(tmp_repo, "linux")
    assert len(discovered) == 1
    assert "files/.vimrc" in discovered[0].src


def test_explicit_overrides_discovered(dots, tmp_repo, tmp_home):
    """Explicit [[file]] overrides discovered entry's dst."""
    (tmp_repo / "files" / ".gitconfig").write_text("[user]")

    discovered = dots.discover_files(tmp_repo, "linux")
    explicit = [
        dots.FileEntry(
            src="files/.gitconfig",
            dst="~/custom/.gitconfig",
            mode="644",
        )
    ]
    merged = dots.merge_file_entries(discovered, explicit)
    assert len(merged) == 1
    assert merged[0].dst == "~/custom/.gitconfig"
    assert merged[0].mode == "644"


def test_explicit_new_src_appended(dots, tmp_repo, tmp_home):
    """Explicit [[file]] with new src appended to discovered."""
    (tmp_repo / "files" / ".gitconfig").write_text("[user]")

    discovered = dots.discover_files(tmp_repo, "linux")
    explicit = [
        dots.FileEntry(
            src="extra/myfile",
            dst="~/myfile",
        )
    ]
    merged = dots.merge_file_entries(discovered, explicit)
    assert len(merged) == 2


def test_ds_store_skipped(dots, tmp_repo, tmp_home):
    """.DS_Store files are skipped."""
    (tmp_repo / "files" / ".DS_Store").write_text("")
    (tmp_repo / "files" / ".vimrc").write_text("set nocompatible")

    discovered = dots.discover_files(tmp_repo, "linux")
    assert len(discovered) == 1
