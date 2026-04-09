"""Tests for error message content for each anticipated case."""

import pytest

from dots.config import load_config
from dots.errors import ConfigError, DotsError, ToolInstallError


def test_dots_error_render():
    err = DotsError("Something failed", hint="Try this instead")
    rendered = err.render()
    assert "✗ Something failed" in rendered
    assert "Try this instead" in rendered


def test_config_error_is_dots_error():
    err = ConfigError("Parse error")
    assert isinstance(err, DotsError)


def test_tool_install_error_is_dots_error():
    err = ToolInstallError("Install failed")
    assert isinstance(err, DotsError)


def test_error_without_hint():
    err = DotsError("Something failed")
    rendered = err.render()
    assert "✗ Something failed" in rendered
    lines = [line.strip() for line in rendered.splitlines() if line.strip()]
    assert len(lines) == 1


def test_toml_parse_error_message(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text("invalid toml [[[")
    with pytest.raises(ConfigError) as exc_info:
        load_config(toml, tmp_repo)
    assert "Failed to parse" in str(exc_info.value)
    assert "TOML reference" in exc_info.value.hint
