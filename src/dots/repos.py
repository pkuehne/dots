"""Git repository cloning and updating."""

from __future__ import annotations

import dots.utils as _utils
from dots.config import RepoEntry
from dots.errors import DotsError
from dots.utils import expand


def clone_repo(r: RepoEntry) -> str:
    dst = expand(r.dst)
    if dst.exists():
        if not (dst / ".git").exists():
            raise DotsError(
                f"Cannot clone {r.name} to {dst}",
                hint="Reason: Directory exists but is not a git repository\n\n"
                "Hint: If you want dots to manage this directory, remove it first:\n"
                f"  rm -rf {dst}\n"
                f"Then re-run: dots repos clone {r.name}\n\n"
                "If you want to keep the existing installation, remove the [[repo]] entry\n"
                "from dots.toml or set a different dst.",
            )
        return "already"
    dst.parent.mkdir(parents=True, exist_ok=True)
    repo_url = r.repo
    if "/" in repo_url and "://" not in repo_url and "@" not in repo_url:
        repo_url = f"https://github.com/{repo_url}"
    cmd = ["git", "clone"]
    if r.shallow:
        cmd += ["--depth", "1"]
    if r.ref:
        cmd += ["--branch", r.ref]
    cmd += [repo_url, str(dst)]
    _utils.run(cmd)
    if r.on_install:
        _utils.run(r.on_install, shell=True, cwd=str(dst))
    return "ok"


def update_repo(r: RepoEntry) -> str:
    dst = expand(r.dst)
    if not dst.exists():
        return "missing"
    if r.shallow:
        _utils.run(["git", "fetch", "--depth", "1"], cwd=str(dst))
        _utils.run(["git", "reset", "--hard", "FETCH_HEAD"], cwd=str(dst))
    else:
        _utils.run(["git", "pull"], cwd=str(dst))
    if r.on_update:
        _utils.run(r.on_update, shell=True, cwd=str(dst))
    return "ok"
