"""SSH config generation."""

from __future__ import annotations

import dots.platform as _plat
from dots.config import Config
from dots.constants import GENERATED_HEADER, SSH_KEYWORD_MAP
from dots.utils import expand


def snake_to_ssh_keyword(key: str) -> str:
    return SSH_KEYWORD_MAP.get(key, "".join(w.capitalize() for w in key.split("_")))


def generate_ssh_config(config: Config) -> str:
    plat = _plat.detect_platform()
    lines = [
        GENERATED_HEADER,
        "# Source: dots.toml [[ssh.host]]",
        "# Regenerate: dots apply",
        "",
    ]

    for host_entry in config.ssh_hosts:
        if host_entry.only and plat not in host_entry.only:
            continue
        lines.append(f"Host {host_entry.host}")
        for key, value in host_entry.options.items():
            keyword = snake_to_ssh_keyword(key)
            if isinstance(value, bool):
                value = "yes" if value else "no"
            lines.append(f"    {keyword} {value}")
        lines.append("")

    return "\n".join(lines) + "\n"


SSH_INCLUDE_LINE = "Include ~/.config/dots/ssh/config"


def ssh_init(config: Config) -> None:
    ssh_dir = expand("~/.ssh")
    ssh_dir.mkdir(mode=0o700, exist_ok=True)
    ssh_config = ssh_dir / "config"

    generated = generate_ssh_config(config)
    out_dir = expand("~/.config/dots/ssh")
    out_dir.mkdir(parents=True, exist_ok=True)
    out_file = out_dir / "config"
    out_file.write_text(generated)
    out_file.chmod(0o600)

    # Insert Include into ~/.ssh/config
    if ssh_config.exists():
        text = ssh_config.read_text()
        if SSH_INCLUDE_LINE in text:
            return
        ssh_config.write_text(SSH_INCLUDE_LINE + "\n\n" + text)
    else:
        ssh_config.write_text(SSH_INCLUDE_LINE + "\n")
    ssh_config.chmod(0o600)
