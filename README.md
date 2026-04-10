# dots

Dotfile management, tool installation, and shell environment generation.
Works on Linux, macOS, and Termux with zero mandatory dependencies.

## Install

**With pipx (recommended):**

```sh
pipx install git+https://github.com/peterkuehne/dots.git
```

**Without pipx:**

```sh
curl -fsSL https://raw.githubusercontent.com/peterkuehne/dots/main/install.sh | sh
```

Or clone and install directly:

```sh
git clone https://github.com/peterkuehne/dots.git
cd dots
python3 -m pip install --user .
```

**Requirements:** Python 3.10+. No third-party packages are required.
Optional: `tomli` (Python < 3.11), `jinja2` (templates), `age` (secrets).

## Quick start

```sh
# Scaffold a new dotfiles repo
dots init ~/dotfiles

# Copy existing dotfiles in
cp ~/.zshrc ~/dotfiles/files/
cp ~/.gitconfig ~/dotfiles/files/

# Or scan for common dotfiles automatically
cd ~/dotfiles && dots migrate --write

# Preview what will happen
dots apply --dry-run

# Deploy symlinks
dots apply
```

That's it. No config file is required — dots discovers files under `files/`
and symlinks them into `~`.

## How it works

Dots uses a layered configuration model:

| Level | What you get |
|-------|-------------|
| **Zero config** | Drop files into `files/`, run `dots apply`, get symlinks into `~` |
| **dots.toml** | Override destinations, add platform guards, set env vars, configure PATH |
| **Managed subsystems** | Let dots own your shell init, git config, or SSH config |
| **Presets** | Opinionated bundles (ejectable to plain files) |

### File discovery

```
~/dotfiles/
  files/             # -> symlinked to ~/
    .zshrc           # -> ~/.zshrc
    .config/
      nvim/
        init.lua     # -> ~/.config/nvim/init.lua
  files.d/
    linux/           # -> only deployed on Linux
      .Xresources
    darwin/          # -> only deployed on macOS
      .Brewfile
  shell/             # -> shell snippets (if shell.managed)
  dots.toml          # -> configuration (optional)
```

Files are handled automatically by suffix:

- `.age` — decrypted with [age](https://github.com/FiloSottile/age), written as a regular file
- `.j2` — rendered as a [Jinja2](https://jinja.palletsprojects.com/) template
- Everything else — symlinked

## Configuration

All configuration lives in `dots.toml`. Every section is optional.

### Environment variables

```toml
[env]
EDITOR = "nvim"
LANG = "en_US.UTF-8"

[[env.when]]
platform = "darwin"
env.HOMEBREW_NO_ANALYTICS = "1"
```

### PATH management

```toml
[shell]
managed = true
path = ["~/.cargo/bin", "~/.local/bin"]
```

### Tool installation

```toml
[tools]
bin_dir = "~/.local/bin"

[[tool]]
name = "rg"
check = "rg --version"
tags = ["core"]

  [[tool.install]]
  method = "apt"
  package = "ripgrep"

  [[tool.install]]
  method = "brew"
  package = "ripgrep"

  [[tool.install]]
  method = "github"
  repo = "BurntSushi/ripgrep"
  asset = "ripgrep-{version}-{arch}-unknown-linux-musl.tar.gz"
  bin = ["rg"]
```

Install methods: `apt`, `brew`, `pkg`, `cargo`, `go`, `pip`, `pipx`,
`npm`, `github`, `script`, `manual`.

### Git config

```toml
[git]
managed = true
user.name = "Your Name"
user.email = "you@example.com"
editor = "nvim"
```

### SSH config

```toml
[ssh]
managed = true

[[ssh.host]]
name = "dev"
hostname = "dev.example.com"
user = "deploy"
identity_file = "~/.ssh/id_ed25519"
```

### Repositories

```toml
[[repo]]
name = "private-config"
url = "git@github.com:you/private-config.git"
dest = "~/code/private-config"
```

### Profiles

Override any config per platform, hostname, or manual flag:

```toml
[profiles.work-laptop]
env.HTTP_PROXY = "http://proxy.corp:8080"
git.user.email = "you@work.com"

[profiles.linux]
shell.path = ["/snap/bin"]
```

### Secrets

```toml
[secrets]
identity = "~/.config/dots/key.txt"
```

Encrypt files with `dots encrypt`, decrypt with `dots decrypt`.
Files ending in `.age` under `files/` are decrypted automatically during apply.

## Commands

| Command | Description |
|---------|-------------|
| `dots init [dir]` | Scaffold a new dots repository |
| `dots apply` | Deploy files, generate configs, clone repos |
| `dots apply --dry-run` | Preview without making changes |
| `dots status` | Show deployment state |
| `dots diff [file]` | Show diffs between source and deployed |
| `dots edit <file>` | Open a managed file in your editor |
| `dots add <path>` | Adopt an existing file into the repo |
| `dots list` | List managed files |
| `dots doctor` | System health check |
| `dots migrate` | Scan for unmanaged dotfiles |
| `dots tools check` | Check which tools are installed |
| `dots tools install` | Install missing tools |
| `dots tools list` | List configured tools |
| `dots shell show` | Print generated shell snippets |
| `dots git init` | Enable managed git config |
| `dots git show` | Print managed gitconfig |
| `dots ssh init` | Enable managed SSH config |
| `dots ssh show` | Print managed SSH config |
| `dots repos clone` | Clone missing repositories |
| `dots repos status` | Show repository states |
| `dots env show` | Print generated env snippet |
| `dots encrypt <file>` | Encrypt a file with age |
| `dots decrypt <file>` | Decrypt an .age file |

All commands are idempotent. Running twice produces the same result.

## Platforms

- **Linux** — full support, `apt` + `github` install methods
- **macOS** — full support, `brew` + `github` install methods
- **Termux** — full support, uses `pkg` instead of `apt`, no sudo
- **WSL** — detected as Linux, use profiles for WSL-specific config

## Development

```sh
git clone https://github.com/peterkuehne/dots.git
cd dots
pip install -e ".[dev]"
pytest tests/
ruff check src/ tests/
```

## License

MIT
