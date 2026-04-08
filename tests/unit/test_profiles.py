"""Tests for profile layering, auto-activation, and deep_merge semantics."""

from unittest.mock import patch

import pytest


def test_platform_profile_auto_activated(dots, tmp_repo):
    """Platform profile auto-activated."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[env]
EDITOR = "vim"

[profiles.linux]
env.EDITOR = "nvim"
""")
    with patch.object(dots, "detect_platform", return_value="linux"), \
         patch.object(dots, "get_hostname", return_value="myhost"):
        config = dots.load_config(toml, tmp_repo)
    assert config.env["EDITOR"] == "nvim"


def test_hostname_profile_auto_activated(dots, tmp_repo):
    """Hostname profile auto-activated."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[env]
EDITOR = "vim"

[profiles.myhost]
env.EDITOR = "code"
""")
    with patch.object(dots, "detect_platform", return_value="linux"), \
         patch.object(dots, "get_hostname", return_value="myhost"):
        config = dots.load_config(toml, tmp_repo)
    assert config.env["EDITOR"] == "code"


def test_manual_profile_highest_priority(dots, tmp_repo):
    """Manual profile has highest priority."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[env]
EDITOR = "vim"

[profiles.linux]
env.EDITOR = "nvim"

[profiles.myhost]
env.EDITOR = "code"

[profiles.work]
env.EDITOR = "emacs"
""")
    with patch.object(dots, "detect_platform", return_value="linux"), \
         patch.object(dots, "get_hostname", return_value="myhost"):
        config = dots.load_config(toml, tmp_repo, profile="work")
    assert config.env["EDITOR"] == "emacs"


def test_layering_order(dots, tmp_repo):
    """Profile layering: global < platform < hostname < manual."""
    toml = tmp_repo / "dots.toml"
    toml.write_text("""\
[env]
ALPHA = "global"
BETA = "global"
GAMMA = "global"
DELTA = "global"

[profiles.linux]
env.BETA = "platform"
env.GAMMA = "platform"
env.DELTA = "platform"

[profiles.myhost]
env.GAMMA = "hostname"
env.DELTA = "hostname"

[profiles.work]
env.DELTA = "manual"
""")
    with patch.object(dots, "detect_platform", return_value="linux"), \
         patch.object(dots, "get_hostname", return_value="myhost"):
        config = dots.load_config(toml, tmp_repo, profile="work")
    assert config.env["ALPHA"] == "global"
    assert config.env["BETA"] == "platform"
    assert config.env["GAMMA"] == "hostname"
    assert config.env["DELTA"] == "manual"


def test_profile_replaces_lists(dots):
    """Profile setting replaces lists, doesn't extend."""
    base = {"shell": {"path": ["/a", "/b"]}}
    override = {"shell": {"path": ["/c"]}}
    result = dots.deep_merge(base, override)
    assert result["shell"]["path"] == ["/c"]


def test_nonexistent_profile_ignored(dots, tmp_repo):
    """Non-existent manual profile is silently ignored."""
    toml = tmp_repo / "dots.toml"
    toml.write_text('[env]\nEDITOR = "vim"\n')
    with patch.object(dots, "detect_platform", return_value="linux"), \
         patch.object(dots, "get_hostname", return_value="myhost"):
        config = dots.load_config(toml, tmp_repo, profile="nonexistent")
    assert config.env["EDITOR"] == "vim"
