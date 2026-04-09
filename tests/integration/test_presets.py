"""Integration tests for fzf/tmux preset generation and eject."""

from dots.presets import TMUX_PRESET, generate_fzf_preset


def test_fzf_preset_content():
    result = generate_fzf_preset()
    assert "command -v fzf" in result
    assert "FZF_DEFAULT_OPTS" in result
    assert "FZF_DEFAULT_COMMAND" in result
    assert "key-bindings" in result


def test_fzf_preset_shell_substitution():
    zsh = generate_fzf_preset(shell_name="zsh")
    bash = generate_fzf_preset(shell_name="bash")
    assert "key-bindings.zsh" in zsh
    assert "key-bindings.bash" in bash


def test_tmux_preset_content():
    assert "prefix C-a" in TMUX_PRESET
    assert "mouse on" in TMUX_PRESET
    assert "tpm" in TMUX_PRESET
    assert "tmux-sensible" in TMUX_PRESET
    assert "tmux-resurrect" in TMUX_PRESET


def test_fzf_eject(tmp_repo):
    shell_dir = tmp_repo / "shell"
    shell_dir.mkdir(exist_ok=True)

    content = generate_fzf_preset()
    (shell_dir / "80-fzf.sh").write_text(content)

    result = (shell_dir / "80-fzf.sh").read_text()
    assert "FZF_DEFAULT_OPTS" in result


def test_tmux_eject(tmp_path):
    dest = tmp_path / ".tmux.conf"
    dest.write_text(TMUX_PRESET)

    assert dest.exists()
    assert "prefix C-a" in dest.read_text()
