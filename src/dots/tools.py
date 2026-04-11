"""Tool installation logic."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

import dots.platform as _plat
import dots.utils as _utils
from dots.config import Tool, ToolInstall
from dots.errors import ToolInstallError


def tool_is_installed(tool: Tool) -> bool:
    try:
        result = subprocess.run(tool.check, shell=True, capture_output=True, text=True)
        return result.returncode == 0
    except Exception:
        return False


def find_install_method(tool: Tool) -> ToolInstall | None:
    plat = _plat.detect_platform()
    managers = {
        "pkg": shutil.which("pkg"),
        "apt": shutil.which("apt-get"),
        "brew": shutil.which("brew"),
        "cargo": shutil.which("cargo"),
        "go": shutil.which("go"),
        "pip": shutil.which("pip3") or shutil.which("pip"),
        "pipx": shutil.which("pipx"),
        "npm": shutil.which("npm"),
        "github": True,  # Always available (uses urllib)
        "script": True,
        "manual": True,
    }
    for inst in tool.install:
        if inst.only and plat not in inst.only:
            continue
        if inst.method in managers and managers[inst.method]:
            return inst
    return None


def github_get_latest_release(repo: str) -> dict:
    url = f"https://api.github.com/repos/{repo}/releases/latest"
    headers = {"Accept": "application/vnd.github.v3+json"}
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"token {token}"
    req = Request(url, headers=headers)
    try:
        with urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode())
    except HTTPError as e:
        if e.code == 403:
            # Rate limit
            reset = e.headers.get("X-RateLimit-Reset", "")
            hint = "GitHub API rate limit exceeded (60 requests/hour for unauthenticated)"
            if reset:
                import time as _time

                try:
                    reset_time = int(reset) - int(_time.time())
                    hint += f"\n\nResets in: {max(1, reset_time // 60)} minutes"
                except (ValueError, TypeError):
                    pass
            hint += (
                "\n\nHint: Set GITHUB_TOKEN to raise the limit to 5000 req/hour:"
                "\n  export GITHUB_TOKEN=ghp_..."
            )
            raise ToolInstallError(
                f"GitHub API rate limit exceeded for {repo}",
                hint=hint,
            )
        raise ToolInstallError(
            f"Failed to reach GitHub API for repo {repo}",
            hint=f"HTTP {e.code}: {e.reason}",
        )
    except URLError as e:
        raise ToolInstallError(
            f"Failed to reach GitHub API for repo {repo}",
            hint=f"Reason: {e.reason}\n\nHints:\n"
            "· Are you behind a proxy? Set: export HTTPS_PROXY=http://proxy:3128\n"
            "· Check connectivity: curl https://api.github.com",
        )


def github_download_asset(url: str, dest: Path) -> None:
    headers = {"Accept": "application/octet-stream"}
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"token {token}"
    req = Request(url, headers=headers)
    with urlopen(req, timeout=120) as resp:
        dest.write_bytes(resp.read())


def install_github(tool: Tool, inst: ToolInstall, bin_dir: Path) -> None:
    release = github_get_latest_release(inst.repo)
    tag = release.get("tag_name", "")
    version = tag.lstrip("v")

    arch = _plat.detect_arch()
    os_name = _plat.detect_os_name()
    goarch = _plat.detect_goarch()

    # Apply per-tool arch name overrides (for tools with mixed naming conventions)
    arch = inst.arch_map.get(arch, arch)

    # Build asset pattern
    asset_pattern = inst.asset or f"{tool.name}-{version}-*"
    asset_pattern = (
        asset_pattern.replace("{version}", version)
        .replace("{arch}", arch)
        .replace("{os}", os_name)
        .replace("{goarch}", goarch)
        .replace("{name}", tool.name)
    )

    # Find matching asset
    assets = release.get("assets", [])
    matched = None
    for a in assets:
        name = a.get("name", "")
        if _glob_match(asset_pattern, name):
            matched = a
            break

    if not matched:
        available = ", ".join(a["name"] for a in assets[:10])
        raise ToolInstallError(
            f"No matching asset for {tool.name} in {inst.repo}@{tag}",
            hint=f"Pattern: {asset_pattern}\nAvailable: {available}",
        )

    # Download
    bin_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        download_name = matched["name"]
        download_path = tmppath / download_name
        github_download_asset(matched["browser_download_url"], download_path)

        binary_name = inst.binary or tool.name
        dest = bin_dir / binary_name

        if download_name.endswith(".tar.gz") or download_name.endswith(".tgz"):
            with tarfile.open(str(download_path), "r:gz") as tf:
                _safe_tar_extractall(tf, tmppath / "extracted")
            _find_and_install_binary(tmppath / "extracted", binary_name, dest, inst.strip)
        elif download_name.endswith(".zip"):
            with zipfile.ZipFile(str(download_path), "r") as zf:
                _safe_zip_extractall(zf, tmppath / "extracted")
            _find_and_install_binary(tmppath / "extracted", binary_name, dest, inst.strip)
        else:
            # Raw binary
            shutil.copy2(str(download_path), str(dest))
            dest.chmod(0o755)


def _path_escapes(member_path: Path, dest: Path) -> bool:
    resolved = dest.resolve()
    return not str(member_path).startswith(str(resolved) + os.sep) and member_path != resolved


_TRAVERSAL_HINT = "The archive contains a path traversal entry. This may be a malicious archive."
_SYMLINK_HINT = "The archive contains an absolute symlink. This may be a malicious archive."


def _safe_tar_extractall(tf: tarfile.TarFile, dest: Path) -> None:
    dest.mkdir(parents=True, exist_ok=True)
    for member in tf.getmembers():
        member_path = (dest / member.name).resolve()
        if _path_escapes(member_path, dest):
            raise ToolInstallError(
                f"Refusing to extract '{member.name}' — path escapes target",
                hint=_TRAVERSAL_HINT,
            )
        if member.issym() or member.islnk():
            if Path(member.linkname).is_absolute():
                raise ToolInstallError(
                    f"Refusing to extract symlink '{member.name}' → "
                    f"'{member.linkname}' — absolute target",
                    hint=_SYMLINK_HINT,
                )
    # Use data filter on Python 3.12+ to strip dangerous metadata
    if hasattr(tarfile, "data_filter"):
        tf.extractall(str(dest), filter="data")
    else:
        tf.extractall(str(dest))


def _safe_zip_extractall(zf: zipfile.ZipFile, dest: Path) -> None:
    dest.mkdir(parents=True, exist_ok=True)
    for info in zf.infolist():
        member_path = (dest / info.filename).resolve()
        if _path_escapes(member_path, dest):
            raise ToolInstallError(
                f"Refusing to extract '{info.filename}' — path escapes target",
                hint=_TRAVERSAL_HINT,
            )
    zf.extractall(str(dest))


def _find_and_install_binary(extract_dir: Path, binary_name: str, dest: Path, strip: int) -> None:
    # Search for the binary in extracted files
    for root_path, dirs, files in os.walk(str(extract_dir)):
        for f in files:
            if f == binary_name or f == binary_name.split("/")[-1]:
                src = Path(root_path) / f
                shutil.copy2(str(src), str(dest))
                dest.chmod(0o755)
                return
    raise ToolInstallError(
        f"Binary '{binary_name}' not found in archive",
        hint="Check the 'binary' field in the install method.",
    )


def _glob_match(pattern: str, name: str) -> bool:
    regex = "^" + re.escape(pattern).replace(r"\*", ".*").replace(r"\?", ".") + "$"
    return bool(re.match(regex, name))


def install_tool(tool: Tool, inst: ToolInstall, bin_dir: Path) -> str:
    plat = _plat.detect_platform()

    if inst.method == "pkg":
        _utils.run(["pkg", "install", "-y", inst.package])
        return "pkg"

    elif inst.method == "apt":
        if plat == "termux":
            raise ToolInstallError(
                "Install method 'apt' requires sudo, which is not available on Termux",
                hint=f"Use 'pkg' instead:\n  dots tools install {tool.name} --method pkg",
            )
        cmd = ["apt-get", "install", "-y", inst.package]
        if os.getuid() != 0:
            cmd = ["sudo"] + cmd
        _utils.run(cmd)
        return "apt"

    elif inst.method == "brew":
        _utils.run(["brew", "install", inst.package])
        return "brew"

    elif inst.method == "cargo":
        cmd = ["cargo", "install", inst.package]
        if inst.binary:
            cmd.extend(["--bin", inst.binary])
        _utils.run(cmd)
        return "cargo"

    elif inst.method == "go":
        package = inst.package
        if not package.endswith("@latest"):
            package += "@latest"
        _utils.run(["go", "install", package])
        return "go"

    elif inst.method == "pip":
        pip = shutil.which("pip3") or shutil.which("pip") or "pip3"
        _utils.run([pip, "install", "--user", inst.package])
        return "pip"

    elif inst.method == "pipx":
        _utils.run(["pipx", "install", inst.package])
        return "pipx"

    elif inst.method == "npm":
        _utils.run(["npm", "install", "-g", inst.package])
        return "npm"

    elif inst.method == "github":
        install_github(tool, inst, bin_dir)
        return "github"

    elif inst.method == "script":
        _utils.run(inst.script, shell=True)
        return "script"

    elif inst.method == "manual":
        print("  Manual install: {}".format(inst.note or "see documentation"))
        return "manual"

    else:
        raise ToolInstallError(
            f"Unknown install method: {inst.method}",
            hint="Supported methods: pkg, apt, brew, cargo, go, pip, pipx, npm,"
            " github, script, manual",
        )
