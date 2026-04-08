"""Tests for TOML parsing, profile layering, and config validation."""

import pytest


def test_minimal_toml_no_sections(dots, tmp_repo):
    """Valid minimal dots.toml with no sections."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("[meta]\nversion = 1\n")
    config = dots.load_config(toml, tmp_repo)
    assert config.meta.version == 1
    assert config.meta.default_mode == "symlink"


def test_all_sections_parsed(dots, full_repo):
    """All sections present and correctly typed."""
    toml = full_repo / "dots.toml"
    config = dots.load_config(toml, full_repo)
    assert config.meta.version == 1
    assert config.git.name == "Test User"
    assert config.git.email == "test@example.com"
    assert config.shell.managed is True
    assert config.ssh.managed is True
    assert len(config.tools) == 3
    assert len(config.repos) == 1
    assert config.env["EDITOR"] == "nvim"


def test_tool_shell_env_attached(dots, full_repo):
    """TOML [tool.shell.env] attached to preceding [[tool]]."""
    config = dots.load_config(full_repo / "dots.toml", full_repo)
    bat = [t for t in config.tools if t.name == "bat"][0]
    assert bat.shell.env == {"BAT_THEME": "ansi"}


def test_tool_shell_init(dots, full_repo):
    """Tool shell init command parsed."""
    config = dots.load_config(full_repo / "dots.toml", full_repo)
    zoxide = [t for t in config.tools if t.name == "zoxide"][0]
    assert "zoxide init" in zoxide.shell.init


def test_profile_override_merges(dots, full_repo):
    """Profile override merges correctly."""
    config = dots.load_config(full_repo / "dots.toml", full_repo, profile="work")
    assert config.env["EDITOR"] == "code"


def test_profile_does_not_modify_unrelated(dots, full_repo):
    """Profile override does not modify unrelated keys."""
    config = dots.load_config(full_repo / "dots.toml", full_repo, profile="work")
    assert config.env["VISUAL"] == "nvim"
    assert config.env["PAGER"] == "less"


def test_uppercase_enforcement(dots, tmp_repo):
    """Lowercase env keys raise error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\neditor = "vim"\n')
    with pytest.raises(dots.ConfigError, match="UPPERCASE"):
        dots.load_config(toml, tmp_repo)


def test_path_in_env_raises(dots, tmp_repo):
    """PATH in [env] raises error with hint."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\nPATH = "/usr/bin"\n')
    with pytest.raises(dots.ConfigError, match="PATH must not appear"):
        dots.load_config(toml, tmp_repo)


def test_duplicate_env_keys_raise(dots, tmp_repo):
    """Duplicate env keys are caught by TOML parser."""
    # TOML spec: duplicate keys are invalid, tomllib raises
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\nEDITOR = "vim"\nEDITOR = "nvim"\n')
    with pytest.raises(dots.ConfigError):
        dots.load_config(toml, tmp_repo)


def test_empty_toml(dots, tmp_repo):
    """Empty dots.toml parses without error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("")
    config = dots.load_config(toml, tmp_repo)
    assert config.meta.version == 1


def test_no_toml_file(dots, tmp_repo):
    """Missing dots.toml returns default config."""
    config = dots.load_config(None, tmp_repo)
    assert config.meta.version == 1
    assert config.shell.managed is False


def test_file_entry_missing_src(dots, tmp_repo):
    """[[file]] without src raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[file]]\ndst = "~/.foo"\n')
    with pytest.raises(dots.ConfigError, match="src"):
        dots.load_config(toml, tmp_repo)


def test_file_entry_missing_dst(dots, tmp_repo):
    """[[file]] without dst raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[file]]\nsrc = "files/.foo"\n')
    with pytest.raises(dots.ConfigError, match="dst"):
        dots.load_config(toml, tmp_repo)


def test_tool_missing_name(dots, tmp_repo):
    """[[tool]] without name raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[tool]]\ndesc = "foo"\n')
    with pytest.raises(dots.ConfigError, match="name"):
        dots.load_config(toml, tmp_repo)


def test_repo_missing_name(dots, tmp_repo):
    """[[repo]] without name raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[repo]]\nrepo = "foo/bar"\ndst = "~/foo"\n')
    with pytest.raises(dots.ConfigError, match="name"):
        dots.load_config(toml, tmp_repo)


def test_deep_merge(dots):
    """deep_merge replaces leaf values, merges dicts."""
    base = {"a": 1, "b": {"c": 2, "d": 3}, "e": [1, 2]}
    over = {"b": {"c": 99}, "e": [3, 4]}
    result = dots.deep_merge(base, over)
    assert result["a"] == 1
    assert result["b"]["c"] == 99
    assert result["b"]["d"] == 3
    assert result["e"] == [3, 4]  # Lists are replaced, not merged


def test_deep_merge_does_not_mutate(dots):
    """deep_merge returns a new dict, doesn't mutate inputs."""
    base = {"a": {"b": 1}}
    over = {"a": {"b": 2}}
    result = dots.deep_merge(base, over)
    assert base["a"]["b"] == 1
    assert result["a"]["b"] == 2


def test_env_when_missing_key(dots, tmp_repo):
    """[[env.when]] without key raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[env.when]]\nvalue = "foo"\n')
    with pytest.raises(dots.ConfigError, match="key"):
        dots.load_config(toml, tmp_repo)


def test_env_when_missing_value(dots, tmp_repo):
    """[[env.when]] without value raises error."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[[env.when]]\nkey = "FOO"\n')
    with pytest.raises(dots.ConfigError, match="value"):
        dots.load_config(toml, tmp_repo)


def test_default_mode_copy(dots, tmp_repo):
    """default_mode = copy is parsed."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[meta]\ndefault_mode = "copy"\n')
    config = dots.load_config(toml, tmp_repo)
    assert config.meta.default_mode == "copy"


def test_secrets_config(dots, full_repo):
    """Secrets config parsed correctly."""
    config = dots.load_config(full_repo / "dots.toml", full_repo)
    assert config.secrets.identity == "~/.config/dots/key.txt"


def test_presets_config(dots, full_repo):
    """Presets default to false."""
    config = dots.load_config(full_repo / "dots.toml", full_repo)
    assert config.presets.fzf is False
    assert config.presets.tmux is False
