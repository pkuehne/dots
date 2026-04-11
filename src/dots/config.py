"""Configuration dataclasses and TOML parsing."""

from __future__ import annotations

import copy
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import dots.platform as _plat
from dots.errors import ConfigError

# ── Optional TOML import ─────────────────────────────────────────────────────

try:
    import tomllib  # Python 3.11+
except ImportError:
    try:
        import tomli as tomllib  # type: ignore[no-redef]
    except ImportError:
        tomllib = None  # type: ignore[assignment]


# ── Dataclasses ──────────────────────────────────────────────────────────────


@dataclass
class MetaConfig:
    version: int = 1
    default_mode: str = "symlink"


@dataclass
class ShellConfig:
    managed: bool = False
    login: bool = False
    zshrc: str = "~/.zshrc"
    bashrc: str = "~/.bashrc"
    dir: str = "~/.config/dots/shell.d"
    path: list[str] = field(default_factory=list)


@dataclass
class GitConfig:
    managed: bool = False
    name: str = ""
    email: str = ""
    editor: str = ""
    default_branch: str = "main"
    pull_rebase: bool = False
    signingkey: str = ""
    sign: bool = False


@dataclass
class SSHConfig:
    managed: bool = False


@dataclass
class SSHHost:
    host: str = ""
    only: list[str] = field(default_factory=list)
    options: dict[str, Any] = field(default_factory=dict)


@dataclass
class ToolsConfig:
    bin_dir: str = "~/.local/bin"


@dataclass
class ToolShell:
    env: dict[str, str] = field(default_factory=dict)
    init: str = ""
    path: list[str] = field(default_factory=list)


@dataclass
class ToolGit:
    pager: bool = False
    diff: bool = False


@dataclass
class ToolInstall:
    method: str = ""
    package: str = ""
    repo: str = ""
    asset: str = ""
    binary: str = ""
    strip: int = 1
    version: str = ""
    script: str = ""
    note: str = ""
    only: list[str] = field(default_factory=list)
    arch_map: dict[str, str] = field(default_factory=dict)


@dataclass
class Tool:
    name: str = ""
    desc: str = ""
    check: str = ""
    tags: list[str] = field(default_factory=list)
    only: list[str] = field(default_factory=list)
    profile: str = ""
    install: list[ToolInstall] = field(default_factory=list)
    shell: ToolShell = field(default_factory=ToolShell)
    git: ToolGit = field(default_factory=ToolGit)


@dataclass
class FileEntry:
    src: str = ""
    dst: str = ""
    only: list[str] = field(default_factory=list)
    profile: str = ""
    template: bool = False
    secret: bool = False
    mode: str = ""
    link: bool | None = None


@dataclass
class RepoEntry:
    name: str = ""
    repo: str = ""
    dst: str = ""
    shallow: bool = False
    ref: str = ""
    on_install: str = ""
    on_update: str = ""
    only: list[str] = field(default_factory=list)
    profile: str = ""


@dataclass
class EnvWhen:
    key: str = ""
    value: str = ""
    if_tool: str = ""
    only: list[str] = field(default_factory=list)


@dataclass
class SecretsConfig:
    recipient: str = ""
    identity: str = "~/.config/dots/key.txt"


@dataclass
class PresetsConfig:
    fzf: bool = False
    tmux: bool = False


@dataclass
class Config:
    meta: MetaConfig = field(default_factory=MetaConfig)
    vars: dict[str, Any] = field(default_factory=dict)
    profiles: dict[str, dict[str, Any]] = field(default_factory=dict)
    env: dict[str, str] = field(default_factory=dict)
    env_when: list[EnvWhen] = field(default_factory=list)
    shell: ShellConfig = field(default_factory=ShellConfig)
    git: GitConfig = field(default_factory=GitConfig)
    ssh: SSHConfig = field(default_factory=SSHConfig)
    ssh_hosts: list[SSHHost] = field(default_factory=list)
    tools_config: ToolsConfig = field(default_factory=ToolsConfig)
    tools: list[Tool] = field(default_factory=list)
    files: list[FileEntry] = field(default_factory=list)
    repos: list[RepoEntry] = field(default_factory=list)
    secrets: SecretsConfig = field(default_factory=SecretsConfig)
    presets: PresetsConfig = field(default_factory=PresetsConfig)
    repo_root: Path = field(default_factory=lambda: Path("."))
    active_profile: str = ""


# ── Parsing ──────────────────────────────────────────────────────────────────


def load_toml(path: Path) -> dict:
    if tomllib is None:
        raise ConfigError(
            "Cannot parse TOML — no parser available",
            hint="Install tomli for Python < 3.11:\n  pip install tomli\n"
            "Or upgrade to Python 3.11+ which includes tomllib.",
        )
    try:
        with open(path, "rb") as f:
            return tomllib.load(f)
    except Exception as e:
        msg = str(e)
        raise ConfigError(
            f"Failed to parse {path.name}",
            hint=f"{msg}" + "\n\nTOML reference: https://toml.io/en/v1.0.0",
        )


def parse_env(raw: dict) -> tuple[dict[str, str], list[EnvWhen]]:
    env_section = raw.get("env", {})
    env = {}
    env_when = []

    when_entries = env_section.pop("when", []) if isinstance(env_section, dict) else []

    for key, value in env_section.items():
        if key == "when":
            continue
        if key == "PATH":
            raise ConfigError(
                "PATH must not appear in [env]",
                hint="Use [shell] path instead:\n"
                '  [shell]\n  path = ["~/.local/bin", "~/.cargo/bin"]',
            )
        if not key.isupper():
            raise ConfigError(
                f"Environment key '{key}' must be UPPERCASE",
                hint=f'Rename to: {key.upper()} = "{value}"',
            )
        if key in env:
            raise ConfigError(
                f"Duplicate environment key: {key}",
                hint="Remove the duplicate entry from [env].",
            )
        env[key] = str(value)

    for entry in when_entries:
        ew = EnvWhen(
            key=entry.get("key", ""),
            value=entry.get("value", ""),
            if_tool=entry.get("if_tool", ""),
            only=entry.get("only", []),
        )
        if not ew.key or not ew.value:
            raise ConfigError(
                "[[env.when]] entry missing required 'key' or 'value'",
                hint="Each [[env.when]] needs at minimum:\n"
                '  key = "VAR_NAME"\n  value = "the value"',
            )
        env_when.append(ew)

    return env, env_when


def parse_tool(raw_tool: dict) -> Tool:
    t = Tool(
        name=raw_tool.get("name", ""),
        desc=raw_tool.get("desc", ""),
        check=raw_tool.get("check", ""),
        tags=raw_tool.get("tags", []),
        only=raw_tool.get("only", []),
        profile=raw_tool.get("profile", ""),
    )
    if not t.name:
        raise ConfigError(
            "[[tool]] entry missing required 'name' field",
            hint='Every tool needs a name:\n  [[tool]]\n  name = "ripgrep"',
        )
    if not t.check:
        t.check = f"which {t.name}"

    for raw_inst in raw_tool.get("install", []):
        inst = ToolInstall(
            method=raw_inst.get("method", ""),
            package=raw_inst.get("package", ""),
            repo=raw_inst.get("repo", ""),
            asset=raw_inst.get("asset", ""),
            binary=raw_inst.get("binary", ""),
            strip=raw_inst.get("strip", 1),
            version=raw_inst.get("version", ""),
            script=raw_inst.get("script", ""),
            note=raw_inst.get("note", ""),
            only=raw_inst.get("only", []),
            arch_map=raw_inst.get("arch_map", {}),
        )
        t.install.append(inst)

    raw_shell = raw_tool.get("shell", {})
    if isinstance(raw_shell, dict):
        t.shell = ToolShell(
            env=raw_shell.get("env", {}),
            init=raw_shell.get("init", ""),
            path=raw_shell.get("path", {}).get("dirs", [])
            if isinstance(raw_shell.get("path"), dict)
            else [],
        )

    raw_git = raw_tool.get("git", {})
    if isinstance(raw_git, dict):
        t.git = ToolGit(
            pager=raw_git.get("pager", False),
            diff=raw_git.get("diff", False),
        )

    return t


def parse_ssh_host(raw: dict) -> SSHHost:
    host = raw.get("host", "")
    only = raw.get("only", [])
    options = {}
    for key, value in raw.items():
        if key in ("host", "only"):
            continue
        options[key] = value
    return SSHHost(host=host, only=only, options=options)


def parse_file_entry(raw: dict) -> FileEntry:
    src = raw.get("src", "")
    dst = raw.get("dst", "")
    if not src:
        raise ConfigError(
            "[[file]] entry missing required 'src' field",
            hint="Every file entry needs:\n  [[file]]\n"
            '  src = "files/.gitconfig"\n  dst = "~/.gitconfig"',
        )
    if not dst:
        raise ConfigError(
            f"[[file]] entry for '{src}' missing required 'dst' field",
            hint='Add a destination:\n  dst = "~/.gitconfig"',
        )
    return FileEntry(
        src=src,
        dst=dst,
        only=raw.get("only", []),
        profile=raw.get("profile", ""),
        template=raw.get("template", False),
        secret=raw.get("secret", False),
        mode=raw.get("mode", ""),
        link=raw.get("link"),
    )


def parse_repo_entry(raw: dict) -> RepoEntry:
    name = raw.get("name", "")
    if not name:
        raise ConfigError(
            "[[repo]] entry missing required 'name' field",
            hint='Every repo needs a name:\n  [[repo]]\n  name = "tpm"',
        )
    return RepoEntry(
        name=name,
        repo=raw.get("repo", ""),
        dst=raw.get("dst", ""),
        shallow=raw.get("shallow", False),
        ref=raw.get("ref", ""),
        on_install=raw.get("on_install", ""),
        on_update=raw.get("on_update", ""),
        only=raw.get("only", []),
        profile=raw.get("profile", ""),
    )


def deep_merge(base: dict, overrides: dict) -> dict:
    result = copy.deepcopy(base)
    for key, value in overrides.items():
        if isinstance(value, dict) and isinstance(result.get(key), dict):
            result[key] = deep_merge(result[key], value)
        else:
            result[key] = copy.deepcopy(value)
    return result


def resolve_profiles(raw: dict, plat: str, hostname: str, manual: str) -> dict:
    profiles = raw.get("profiles", {})
    result = copy.deepcopy(raw)

    for profile_key in [plat, hostname, manual]:
        if not profile_key:
            continue
        overrides = profiles.get(profile_key, {})
        if overrides:
            result = deep_merge(result, overrides)

    return result


def load_config(
    toml_path: Path | None = None,
    repo_root: Path | None = None,
    profile: str = "",
) -> Config:
    if repo_root is None:
        repo_root = Path(".")

    config = Config(repo_root=repo_root)

    if toml_path is None or not toml_path.exists():
        return config

    raw = load_toml(toml_path)

    plat = _plat.detect_platform()
    hostname = _plat.get_hostname()
    resolved = resolve_profiles(raw, plat, hostname, profile)
    config.active_profile = profile

    meta = resolved.get("meta", {})
    config.meta = MetaConfig(
        version=meta.get("version", 1),
        default_mode=meta.get("default_mode", "symlink"),
    )

    config.vars = resolved.get("vars", {})
    config.profiles = raw.get("profiles", {})
    config.env, config.env_when = parse_env(resolved)

    shell = resolved.get("shell", {})
    config.shell = ShellConfig(
        managed=shell.get("managed", False),
        login=shell.get("login", False),
        zshrc=shell.get("zshrc", "~/.zshrc"),
        bashrc=shell.get("bashrc", "~/.bashrc"),
        dir=shell.get("dir", "~/.config/dots/shell.d"),
        path=shell.get("path", []),
    )

    git = resolved.get("git", {})
    config.git = GitConfig(
        managed=git.get("managed", False),
        name=git.get("name", ""),
        email=git.get("email", ""),
        editor=git.get("editor", ""),
        default_branch=git.get("default_branch", "main"),
        pull_rebase=git.get("pull_rebase", False),
        signingkey=git.get("signingkey", ""),
        sign=git.get("sign", False),
    )

    ssh = resolved.get("ssh", {})
    config.ssh = SSHConfig(managed=ssh.get("managed", False))

    for raw_host in resolved.get("ssh", {}).get("host", []):
        config.ssh_hosts.append(parse_ssh_host(raw_host))

    tools_raw = resolved.get("tools", {})
    config.tools_config = ToolsConfig(
        bin_dir=tools_raw.get("bin_dir", "~/.local/bin"),
    )

    for raw_tool in resolved.get("tool", []):
        config.tools.append(parse_tool(raw_tool))

    for raw_file in resolved.get("file", []):
        config.files.append(parse_file_entry(raw_file))

    for raw_repo in resolved.get("repo", []):
        config.repos.append(parse_repo_entry(raw_repo))

    secrets = resolved.get("secrets", {})
    config.secrets = SecretsConfig(
        recipient=secrets.get("recipient", ""),
        identity=secrets.get("identity", "~/.config/dots/key.txt"),
    )

    presets = resolved.get("presets", {})
    config.presets = PresetsConfig(
        fzf=presets.get("fzf", False),
        tmux=presets.get("tmux", False),
    )

    return config
