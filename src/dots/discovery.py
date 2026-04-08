"""File discovery from files/ and files.d/ directories."""

from __future__ import annotations

import os
from pathlib import Path
from typing import List

from dots.config import FileEntry
from dots.utils import should_skip


def discover_files(repo_root: Path, plat: str) -> List[FileEntry]:
    discovered = []
    home = Path.home()

    # Walk files/
    files_dir = repo_root / "files"
    if files_dir.is_dir():
        for root_path, dirs, filenames in os.walk(str(files_dir)):
            root_p = Path(root_path)
            dirs[:] = sorted(d for d in dirs if not should_skip(d))
            for fname in sorted(filenames):
                if should_skip(fname):
                    continue
                src = root_p / fname
                rel = src.relative_to(files_dir)
                dst = home / rel
                entry = FileEntry(
                    src=str(src.relative_to(repo_root)),
                    dst=str(dst),
                )
                # Auto-detect
                if fname.endswith(".age"):
                    entry.secret = True
                    entry.dst = str(dst.with_name(fname[:-4]))
                elif fname.endswith(".j2"):
                    entry.template = True
                    entry.dst = str(dst.with_name(fname[:-3]))
                discovered.append(entry)

    # Walk files.d/{platform}/
    platform_dir = repo_root / "files.d" / plat
    if platform_dir.is_dir():
        for root_path, dirs, filenames in os.walk(str(platform_dir)):
            root_p = Path(root_path)
            dirs[:] = sorted(d for d in dirs if not should_skip(d))
            for fname in sorted(filenames):
                if should_skip(fname):
                    continue
                src = root_p / fname
                rel = src.relative_to(platform_dir)
                dst = home / rel
                entry = FileEntry(
                    src=str(src.relative_to(repo_root)),
                    dst=str(dst),
                    only=[plat],
                )
                if fname.endswith(".age"):
                    entry.secret = True
                    entry.dst = str(dst.with_name(fname[:-4]))
                elif fname.endswith(".j2"):
                    entry.template = True
                    entry.dst = str(dst.with_name(fname[:-3]))
                discovered.append(entry)

    return discovered


def merge_file_entries(
    discovered: List[FileEntry], explicit: List[FileEntry]
) -> List[FileEntry]:
    result = list(discovered)
    discovered_srcs = {e.src for e in discovered}

    for exp in explicit:
        found = False
        for i, disc in enumerate(result):
            if disc.src == exp.src:
                result[i] = exp
                found = True
                break
        if not found:
            result.append(exp)

    return result
