"""Tests for TOML parsing, profile layering, and config validation."""

import pytest

from dots.config import deep_merge, load_config
from dots.errors import ConfigError


def test_minimal_toml_no_sections(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text("[meta]\nversion = 1\n")
    config = load_config(toml, tmp_repo)
    assert config.meta.version == 1
    assert config.meta.default_mode == "symlink"


def test_all_sections_parsed(full_repo):
    toml = full_repo / "dots.toml"
    config = load_config(toml, full_repo)
    assert config.meta.version == 1
    assert config.git.name == "Test User"
    assert config.git.email == "test@example.com"
    assert config.shell.managed is True
    assert config.ssh.managed is True
    assert len(config.tools) == 3
    assert len(config.repos) == 1
    assert config.env["EDITOR"] == "nvim"


def test_tool_shell_env_attached(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo)
    bat = [t for t in config.tools if t.name == "bat"][0]
    assert bat.shell.env == {"BAT_THEME": "ansi"}


def test_tool_shell_init(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo)
    zoxide = [t for t in config.tools if t.name == "zoxide"][0]
    assert "zoxide init" in zoxide.shell.init


def test_profile_override_merges(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo, profile="work")
    assert config.env["EDITOR"] == "code"


def test_profile_does_not_modify_unrelated(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo, profile="work")
    assert config.env["VISUAL"] == "nvim"
    assert config.env["PAGER"] == "less"


def test_uppercase_enforcement(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\neditor = "vim"\n')
    with pytest.raises(ConfigError, match="UPPERCASE"):
        load_config(toml, tmp_repo)


def test_path_in_env_raises(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\nPATH = "/usr/bin"\n')
    with pytest.raises(ConfigError, match="PATH must not appear"):
        load_config(toml, tmp_repo)


def test_duplicate_env_keys_raise(tmp_repo):
    # TOML spec: duplicate keys are invalid, tomllib raises
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\nEDITOR = "vim"\nEDITOR = "nvim"\n')
    with pytest.raises(ConfigError):
        load_config(toml, tmp_repo)


def test_empty_toml(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text("")
    config = load_config(toml, tmp_repo)
    assert config.meta.version == 1


def test_no_toml_file(tmp_repo):
    config = load_config(None, tmp_repo)
    assert config.meta.version == 1
    assert config.shell.managed is False


def test_file_entry_missing_src(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[file]]\ndst = "~/.foo"\n')
    with pytest.raises(ConfigError, match="src"):
        load_config(toml, tmp_repo)


def test_file_entry_missing_dst(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[file]]\nsrc = "files/.foo"\n')
    with pytest.raises(ConfigError, match="dst"):
        load_config(toml, tmp_repo)


def test_tool_missing_name(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[tool]]\ndesc = "foo"\n')
    with pytest.raises(ConfigError, match="name"):
        load_config(toml, tmp_repo)


def test_repo_missing_name(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[repo]]\nrepo = "foo/bar"\ndst = "~/foo"\n')
    with pytest.raises(ConfigError, match="name"):
        load_config(toml, tmp_repo)


def test_deep_merge():
    base = {"a": 1, "b": {"c": 2, "d": 3}, "e": [1, 2]}
    over = {"b": {"c": 99}, "e": [3, 4]}
    result = deep_merge(base, over)
    assert result["a"] == 1
    assert result["b"]["c"] == 99
    assert result["b"]["d"] == 3
    assert result["e"] == [3, 4]  # Lists are replaced, not merged


def test_deep_merge_does_not_mutate():
    base = {"a": {"b": 1}}
    over = {"a": {"b": 2}}
    result = deep_merge(base, over)
    assert base["a"]["b"] == 1
    assert result["a"]["b"] == 2


def test_env_when_missing_key(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[env.when]]\nvalue = "foo"\n')
    with pytest.raises(ConfigError, match="key"):
        load_config(toml, tmp_repo)


def test_env_when_missing_value(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[env.when]]\nkey = "FOO"\n')
    with pytest.raises(ConfigError, match="value"):
        load_config(toml, tmp_repo)


def test_default_mode_copy(tmp_repo):
    toml = tmp_repo / "dots.toml"
    toml.write_text('[meta]\ndefault_mode = "copy"\n')
    config = load_config(toml, tmp_repo)
    assert config.meta.default_mode == "copy"


def test_secrets_config(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo)
    assert config.secrets.identity == "~/.config/dots/key.txt"


def test_presets_config(full_repo):
    config = load_config(full_repo / "dots.toml", full_repo)
    assert config.presets.fzf is False
    assert config.presets.tmux is False
