# Platform Notes

## Termux

- Platform detected via `/data/data/com.termux` directory
- No `sudo` available — `apt` method will error; use `pkg` instead
- `pkg` is the native package manager
- Home directory is `/data/data/com.termux/files/home`
- Some tools may need ARM binaries (`aarch64` or `armv7`)

## Linux

- Standard platform, all features supported
- `apt` method uses `sudo` if not running as root
- GitHub releases prefer `*-unknown-linux-musl` for portability

## macOS

- Platform: `darwin`
- Homebrew (`brew`) is the primary package manager
- GitHub releases use `*-apple-darwin` assets
- Some paths differ: no `/usr/share/doc/fzf/`, fzf lives in brew prefix

## WSL (Windows Subsystem for Linux)

- Detected as `linux` (not a separate platform)
- Use profiles to handle WSL-specific config:
  ```toml
  [profiles.wsl-machine]
  env.DISPLAY = ":0"
  ```
