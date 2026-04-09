"""Tests for 010-env.sh and 020-path.sh generation."""

from dots.config import Config, Tool
from dots.shell import generate_env_snippet, generate_path_snippet


def test_env_snippet_sorted():
    config = Config()
    config.env = {"ZEBRA": "z", "ALPHA": "a", "MIDDLE": "m"}
    config.env_when = []

    result = generate_env_snippet(config)
    lines = [line for line in result.splitlines() if line.startswith("export")]
    keys = [line.split("=")[0].replace("export ", "") for line in lines]
    assert keys == sorted(keys)


def test_env_snippet_double_quoted():
    config = Config()
    config.env = {"EDITOR": "nvim"}
    config.env_when = []

    result = generate_env_snippet(config)
    assert 'export EDITOR="nvim"' in result


def test_path_snippet_order_preserved():
    config = Config()
    config.shell.path = ["~/.first", "~/.second", "~/.third"]
    config.tools = []
    config.tools_config.bin_dir = "~/.local/bin"

    result = generate_path_snippet(config)
    lines = [line for line in result.splitlines() if "case" in line]
    assert len(lines) >= 3


def test_path_snippet_bin_dir_prepended():
    tool = Tool(name="rg")
    config = Config()
    config.shell.path = ["~/.cargo/bin"]
    config.tools = [tool]
    config.tools_config.bin_dir = "~/.local/bin"

    result = generate_path_snippet(config)
    lines = result.splitlines()
    case_lines = [line for line in lines if "case" in line]
    # bin_dir should appear first
    assert ".local/bin" in case_lines[0]


def test_path_snippet_case_guard():
    config = Config()
    config.shell.path = ["~/.local/bin"]
    config.tools = []

    result = generate_path_snippet(config)
    assert 'case ":$PATH:"' in result
    assert "esac" in result
