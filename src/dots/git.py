"""Git config generation."""

from __future__ import annotations

import shutil

from dots.config import Config
from dots.constants import GENERATED_HEADER, MARKER_END, MARKER_START


def generate_gitconfig(config: Config) -> str:
    lines = [
        GENERATED_HEADER,
        "# Source: dots.toml [git] + [[tool]] contributions",
        "# Regenerate: dots apply",
        "",
    ]

    if config.git.name or config.git.email:
        lines.append("[user]")
        if config.git.name:
            lines.append("    name = {}".format(config.git.name))
        if config.git.email:
            lines.append("    email = {}".format(config.git.email))
        if config.git.signingkey:
            lines.append("    signingkey = {}".format(config.git.signingkey))
        lines.append("")

    # Core
    core_lines = []
    if config.git.editor:
        core_lines.append("    editor = {}".format(config.git.editor))
    # Check for delta pager contribution
    for tool in config.tools:
        if tool.name == "delta" and tool.git.pager:
            if shutil.which("delta"):
                core_lines.append("    pager = delta")
    if core_lines:
        lines.append("[core]")
        lines.extend(core_lines)
        lines.append("")

    lines.append("[init]")
    lines.append("    defaultBranch = {}".format(config.git.default_branch))
    lines.append("")

    lines.append("[pull]")
    lines.append("    rebase = {}".format("true" if config.git.pull_rebase else "false"))
    lines.append("")

    if config.git.sign:
        lines.append("[commit]")
        lines.append("    gpgsign = true")
        lines.append("")

    # Tool contributions: delta diff
    for tool in config.tools:
        if tool.name == "delta" and tool.git.diff:
            if shutil.which("delta"):
                lines.append("[diff]")
                lines.append("    tool = delta")
                lines.append("")
                lines.append('[difftool "delta"]')
                lines.append('    cmd = delta "$LOCAL" "$REMOTE"')
                lines.append("")

    return "\n".join(lines) + "\n"


GIT_INCLUDE_BLOCK = """\
{marker_start}
[include]
    path = ~/.config/dots/git/managed.gitconfig
{marker_end}""".format(marker_start=MARKER_START, marker_end=MARKER_END)
