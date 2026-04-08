"""Tests for Jinja2 rendering and template context."""

import os
from pathlib import Path
from unittest.mock import patch

import pytest

jinja2 = pytest.importorskip("jinja2")


def test_template_context_has_platform(dots, tmp_repo, tmp_home):
    """Template context includes platform info."""
    config = dots.Config(repo_root=tmp_repo)
    config.vars = {"name": "Test"}
    config.env = {"EDITOR": "vim"}

    with patch("dots.platform.detect_platform", return_value="linux"):
        ctx = dots.build_template_context(config)

    assert ctx["platform"] == "linux"
    assert ctx["is_linux"] is True
    assert ctx["is_mac"] is False
    assert ctx["is_termux"] is False
    assert ctx["name"] == "Test"
    assert ctx["EDITOR"] == "vim"
    assert "home" in ctx
    assert "hostname" in ctx


def test_template_render_basic(dots, tmp_repo, tmp_home):
    """Basic template rendering works."""
    tmpl = tmp_repo / "test.sh.j2"
    tmpl.write_text("export NAME={{ name }}")

    config = dots.Config(repo_root=tmp_repo)
    config.vars = {"name": "TestUser"}
    config.env = {}

    result = dots.render_template(tmpl, config)
    assert result == "export NAME=TestUser"


def test_template_render_platform_conditional(dots, tmp_repo, tmp_home):
    """Template can use platform conditionals."""
    tmpl = tmp_repo / "test.sh.j2"
    tmpl.write_text("{% if is_linux %}linux{% else %}other{% endif %}")

    config = dots.Config(repo_root=tmp_repo)
    config.vars = {}
    config.env = {}

    with patch("dots.platform.detect_platform", return_value="linux"):
        result = dots.render_template(tmpl, config)
    assert result == "linux"


def test_template_sandbox_blocks_introspection(dots, tmp_repo, tmp_home):
    """Sandbox prevents access to Python internals via template."""
    tmpl = tmp_repo / "evil.j2"
    tmpl.write_text("{{ ''.__class__.__mro__[1].__subclasses__() }}")

    config = dots.Config(repo_root=tmp_repo)
    config.vars = {}
    config.env = {}

    with pytest.raises(Exception):
        dots.render_template(tmpl, config)


def test_template_undefined_var_raises(dots, tmp_repo, tmp_home):
    """Undefined variable raises DotsError with hint."""
    tmpl = tmp_repo / "test.sh.j2"
    tmpl.write_text("{{ nonexistent_var }}")

    config = dots.Config(repo_root=tmp_repo)
    config.vars = {"name": "Test"}
    config.env = {}

    with pytest.raises(dots.DotsError, match="Template error"):
        dots.render_template(tmpl, config)
