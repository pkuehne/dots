"""Tests for error message content for each anticipated case."""

import pytest


def test_dots_error_render(dots):
    """DotsError renders with hint."""
    err = dots.DotsError("Something failed", hint="Try this instead")
    rendered = err.render()
    assert "✗ Something failed" in rendered
    assert "Try this instead" in rendered


def test_config_error_is_dots_error(dots):
    """ConfigError is a subclass of DotsError."""
    err = dots.ConfigError("Parse error")
    assert isinstance(err, dots.DotsError)


def test_deploy_error_is_dots_error(dots):
    """DeployError is a subclass of DotsError."""
    err = dots.DeployError("Deploy failed")
    assert isinstance(err, dots.DotsError)


def test_tool_install_error_is_dots_error(dots):
    """ToolInstallError is a subclass of DotsError."""
    err = dots.ToolInstallError("Install failed")
    assert isinstance(err, dots.DotsError)


def test_error_without_hint(dots):
    """Error renders without hint line when no hint provided."""
    err = dots.DotsError("Something failed")
    rendered = err.render()
    assert "✗ Something failed" in rendered
    lines = [l.strip() for l in rendered.splitlines() if l.strip()]
    assert len(lines) == 1


def test_toml_parse_error_message(dots, tmp_repo):
    """TOML parse error includes helpful message."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("invalid toml [[[")
    with pytest.raises(dots.ConfigError) as exc_info:
        dots.load_config(toml, tmp_repo)
    assert "Failed to parse" in str(exc_info.value)
    assert "TOML reference" in exc_info.value.hint
