"""Shell snippet generation and bootstrapper management."""

from __future__ import annotations

import re
from pathlib import Path

import dots.platform as _plat
from dots.config import Config, Tool
from dots.constants import GENERATED_HEADER, MARKER_END, MARKER_START
from dots.utils import expand


def generate_env_snippet(config: Config) -> str:
    plat = _plat.detect_platform()
    lines = [
        GENERATED_HEADER,
        "# Source: [env] + [[env.when]]",
        "# Regenerate: dots apply",
        "",
    ]
    for key, value in sorted(config.env.items()):
        lines.append(f'export {key}="{value}"')

    for ew in config.env_when:
        if ew.only and plat not in ew.only:
            continue
        if ew.if_tool:
            lines.append("")
            lines.append(f'command -v {ew.if_tool} >/dev/null 2>&1 && export {ew.key}="{ew.value}"')
        else:
            lines.append(f'export {ew.key}="{ew.value}"')

    return "\n".join(lines) + "\n"


def generate_path_snippet(config: Config) -> str:
    lines = [
        GENERATED_HEADER,
        "# Source: [shell] path + [[tool]] shell.path",
        "# Regenerate: dots apply",
        "",
    ]

    paths = list(config.shell.path)

    # Add tool-declared paths
    for tool in config.tools:
        for p in tool.shell.path:
            if p not in paths:
                paths.append(p)

    # Add bin_dir if tools configured
    if config.tools:
        bin_dir = config.tools_config.bin_dir
        if bin_dir not in paths:
            paths.insert(0, bin_dir)

    # Deduplicate preserving order
    seen = set()
    deduped = []
    for p in paths:
        if p not in seen:
            seen.add(p)
            deduped.append(p)

    for p in deduped:
        expanded = expand(p)
        lines.append(
            f'case ":$PATH:" in *":{expanded}:"*) ;; *) export PATH="{expanded}:$PATH" ;; esac'
        )

    return "\n".join(lines) + "\n"


def generate_tool_snippet(tool: Tool, shell_name: str = "zsh") -> str:
    lines = [
        GENERATED_HEADER,
        f"# Source: [[tool]] {tool.name} shell.*",
        "# Regenerate: dots apply",
        "",
        "command -v {} >/dev/null 2>&1 || return".format(
            tool.check.split()[0] if tool.check.startswith("which ") else tool.name
        ),
        "",
    ]

    for key, value in sorted(tool.shell.env.items()):
        lines.append(f'export {key}="{value}"')

    if tool.shell.init:
        init_cmd = tool.shell.init.replace("{shell}", shell_name)
        lines.append("")
        lines.append(init_cmd)

    return "\n".join(lines) + "\n"


def generate_custom_snippet(repo_root: Path) -> str | None:
    zshrc = repo_root / "files" / ".zshrc"
    if not zshrc.exists():
        return None
    content = zshrc.read_text()
    lines = [
        GENERATED_HEADER,
        "# Source: files/.zshrc (migration aid)",
        "# Regenerate: dots apply",
        "",
        content.rstrip(),
    ]
    return "\n".join(lines) + "\n"


# ── Shell Bootstrapper ──────────────────────────────────────────────────────

ZSH_BOOTSTRAPPER = f"""\
{MARKER_START}
_dots_d="${{XDG_CONFIG_HOME:-$HOME/.config}}/dots/shell.d"
if [[ -d "$_dots_d" ]]; then
  for _dots_f in "$_dots_d"/[0-9]*.sh "$_dots_d"/[0-9]*.zsh; do
    [[ -f "$_dots_f" ]] && source "$_dots_f"
  done
  unset _dots_f _dots_d
fi
{MARKER_END}"""

BASH_BOOTSTRAPPER = f"""\
{MARKER_START}
_dots_d="${{XDG_CONFIG_HOME:-$HOME/.config}}/dots/shell.d"
if [ -d "$_dots_d" ]; then
  for _dots_f in "$_dots_d"/[0-9]*.sh "$_dots_d"/[0-9]*.bash; do
    [ -f "$_dots_f" ] && . "$_dots_f"
  done
  unset _dots_f _dots_d
fi
{MARKER_END}"""


def idempotent_insert(path: Path, content: str) -> bool:
    if not path.exists():
        path.write_text(content + "\n")
        return True

    text = path.read_text()

    if MARKER_START in text:
        # Replace existing block
        pattern = re.escape(MARKER_START) + r".*?" + re.escape(MARKER_END)
        new_text = re.sub(pattern, content, text, flags=re.DOTALL)
        if new_text != text:
            path.write_text(new_text)
            return True
        return False

    # Append
    if not text.endswith("\n"):
        text += "\n"
    text += "\n" + content + "\n"
    path.write_text(text)
    return True


def remove_marker_block(path: Path) -> bool:
    if not path.exists():
        return False
    text = path.read_text()
    if MARKER_START not in text:
        return False
    pattern = r"\n?" + re.escape(MARKER_START) + r".*?" + re.escape(MARKER_END) + r"\n?"
    new_text = re.sub(pattern, "\n", text, flags=re.DOTALL)
    path.write_text(new_text)
    return True
