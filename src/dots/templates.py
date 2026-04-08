"""Template context building and Jinja2 rendering."""

from __future__ import annotations

import os
from pathlib import Path

from dots.config import Config
from dots.errors import DotsError
import dots.platform as _plat

try:
    import jinja2  # type: ignore[import-untyped]
except ImportError:
    jinja2 = None  # type: ignore[assignment]


def build_template_context(config: Config) -> dict:
    plat = _plat.detect_platform()
    ctx = {
        "platform": plat,
        "hostname": _plat.get_hostname(),
        "home": str(Path.home()),
        "is_termux": plat == "termux",
        "is_linux": plat in ("linux", "termux"),
        "is_mac": plat == "darwin",
        "is_windows": plat == "windows",
        "profile": config.active_profile,
        "env": dict(os.environ),
    }
    ctx.update(config.vars)
    ctx.update(config.env)
    return ctx


def render_template(src: Path, config: Config) -> str:
    if jinja2 is None:
        raise DotsError(
            "Cannot render template {} — jinja2 not installed".format(src.name),
            hint="Install jinja2:\n  pip install jinja2",
        )
    ctx = build_template_context(config)
    try:
        template_str = src.read_text()
        env = jinja2.Environment(undefined=jinja2.StrictUndefined)
        tmpl = env.from_string(template_str)
        return tmpl.render(**ctx)
    except jinja2.UndefinedError as e:
        available = ", ".join(sorted(ctx.keys()))
        raise DotsError(
            "Template error in {}".format(src),
            hint="Reason: {}\n\nAvailable vars: {}".format(e, available),
        )
