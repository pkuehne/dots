"""Path utilities and shell execution."""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
from pathlib import Path

from dots.constants import SENSITIVE_DIRS, SKIP_NAMES, SKIP_SUFFIXES
from dots.errors import DotsError


def expand(path: str) -> Path:
    return Path(os.path.expandvars(os.path.expanduser(path)))


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def backup(path: Path) -> Path | None:
    if not path.exists() and not path.is_symlink():
        return None
    bak = path.with_suffix(path.suffix + ".dots-bak")
    if path.is_symlink():
        link_target = os.readlink(str(path))
        if bak.exists() or bak.is_symlink():
            bak.unlink()
        bak.symlink_to(link_target)
    else:
        shutil.copy2(str(path), str(bak))
    return bak


def ensure_parent(path: Path) -> None:
    parent = path.parent
    parts = parent.parts
    current = Path(parts[0]) if parts else Path(".")
    for part in parts[1:]:
        current = current / part
        mode = SENSITIVE_DIRS.get(part)
        if mode is not None:
            if not current.exists():
                current.mkdir(mode=mode)
            else:
                try:
                    current.chmod(mode)
                except OSError:
                    pass
        else:
            current.mkdir(exist_ok=True)


def should_skip(name: str) -> bool:
    if name in SKIP_NAMES:
        return True
    if any(name.endswith(s) for s in SKIP_SUFFIXES):
        return True
    if name.endswith("~"):
        return True
    return False


def run(cmd, shell=False, cwd=None, capture=True, check=True, env=None):
    try:
        result = subprocess.run(
            cmd,
            shell=shell,
            cwd=cwd,
            capture_output=capture,
            text=True,
            check=check,
            env=env,
        )
        return result
    except FileNotFoundError:
        binary = cmd[0] if isinstance(cmd, list) else cmd.split()[0]
        raise DotsError(
            f"Command not found: {binary}",
            hint=f"Make sure '{binary}' is installed and on your PATH.",
        )
    except subprocess.CalledProcessError as e:
        msg = "Command failed: {}".format(" ".join(cmd) if isinstance(cmd, list) else cmd)
        stderr = e.stderr.strip() if e.stderr else ""
        hint = f"Exit code: {e.returncode}"
        if stderr:
            hint += "\n" + stderr
        raise DotsError(msg, hint=hint)
