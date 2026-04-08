"""File deployment logic."""

from __future__ import annotations

import shutil
from pathlib import Path
from typing import List

from dots.config import Config, FileEntry
import dots.platform as _plat
from dots.secrets import decrypt_file
from dots.templates import render_template
from dots.utils import backup, ensure_parent, expand, sha256_file


def matches_platform(only: List[str], plat: str) -> bool:
    if not only:
        return True
    return plat in only


def deploy_file(
    entry: FileEntry,
    config: Config,
    dry_run: bool = False,
    force_copy: bool = False,
) -> str:
    plat = _plat.detect_platform()

    if not matches_platform(entry.only, plat):
        return "SKIP"

    if entry.profile and entry.profile != config.active_profile:
        return "SKIP"

    repo_root = config.repo_root
    src = repo_root / entry.src
    dst = Path(expand(entry.dst)) if "~" in entry.dst or "$" in entry.dst else Path(entry.dst)

    if not src.exists():
        return "MISS"

    if dry_run:
        if entry.secret:
            return "DECRYPT → {}".format(dst)
        if entry.template:
            return "RENDER → {}".format(dst)
        mode_str = "copy" if (force_copy or entry.link is False) else "symlink"
        if entry.link is True:
            mode_str = "symlink"
        return "{} → {}".format(mode_str.upper(), dst)

    ensure_parent(dst)

    # Secret: decrypt and write
    if entry.secret:
        identity = expand(config.secrets.identity)
        data = decrypt_file(src, identity)
        if dst.exists() or dst.is_symlink():
            existing = dst.read_bytes() if dst.exists() and not dst.is_symlink() else b""
            if existing == data:
                _apply_mode(dst, entry.mode)
                return "OK"
            backup(dst)
            if dst.is_symlink():
                dst.unlink()
        dst.write_bytes(data)
        _apply_mode(dst, entry.mode or "600")
        return "OK"

    # Template: render and write
    if entry.template:
        rendered = render_template(src, config)
        rendered_bytes = rendered.encode()
        if dst.exists() and not dst.is_symlink():
            if dst.read_bytes() == rendered_bytes:
                _apply_mode(dst, entry.mode)
                return "OK"
            backup(dst)
        elif dst.is_symlink():
            backup(dst)
            dst.unlink()
        dst.write_text(rendered)
        _apply_mode(dst, entry.mode)
        return "OK"

    # Determine link vs copy
    use_symlink = True
    if force_copy or config.meta.default_mode == "copy":
        use_symlink = False
    if entry.link is True:
        use_symlink = True
    elif entry.link is False:
        use_symlink = False

    if use_symlink:
        target = src.resolve()
        if dst.is_symlink():
            if dst.resolve() == target:
                return "LINK"
            # Stale symlink
            backup(dst)
            dst.unlink()
        elif dst.exists():
            backup(dst)
            dst.unlink()
        dst.symlink_to(target)
        return "LINK"
    else:
        if dst.exists() and not dst.is_symlink():
            if sha256_file(dst) == sha256_file(src):
                _apply_mode(dst, entry.mode)
                return "OK"
            backup(dst)
        elif dst.is_symlink():
            backup(dst)
            dst.unlink()
        shutil.copy2(str(src), str(dst))
        _apply_mode(dst, entry.mode)
        return "OK"


def _apply_mode(path: Path, mode_str: str) -> None:
    if not mode_str:
        return
    try:
        path.chmod(int(mode_str, 8))
    except (ValueError, OSError):
        pass


def get_file_state(entry: FileEntry, config: Config) -> str:
    plat = _plat.detect_platform()
    if not matches_platform(entry.only, plat):
        return "SKIP"
    if entry.profile and entry.profile != config.active_profile:
        return "SKIP"

    repo_root = config.repo_root
    src = repo_root / entry.src
    dst = Path(expand(entry.dst)) if "~" in entry.dst or "$" in entry.dst else Path(entry.dst)

    if not dst.exists() and not dst.is_symlink():
        return "MISS"

    if entry.secret or entry.template:
        return "OK"  # Can't easily diff without decrypting/rendering

    if dst.is_symlink():
        if dst.resolve() == src.resolve():
            return "LINK"
        return "DIFF"

    if dst.exists() and src.exists():
        if sha256_file(dst) == sha256_file(src):
            return "OK"
        return "DIFF"

    return "MISS"
