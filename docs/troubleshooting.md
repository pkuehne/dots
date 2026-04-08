# Troubleshooting

## dots.toml not found

dots walks up from the current directory looking for `dots.toml` or `files/`. Run from your dotfiles directory, or use `--repo`:

```
dots --repo ~/dotfiles apply
```

Or set the environment variable: `export DOTS_REPO=~/dotfiles`

## GitHub API rate limit

Unauthenticated requests are limited to 60/hour. Set a token:

```
export GITHUB_TOKEN=ghp_...
```

## Permission denied on ~/.ssh or ~/.gnupg

dots creates these with restrictive permissions (700). If you see errors:

```
ls -la ~/.ssh/
# Expected: drwx------ (700)
chmod 700 ~/.ssh
chmod 600 ~/.ssh/*
```

## Template variable undefined

Check available variables with `dots env show`. Add missing vars to `[vars]` in dots.toml.

## age not found

Install age: `dots tools install age` or download from https://github.com/FiloSottile/age/releases

## Proxy issues with GitHub

Set proxy environment variables:
```
export HTTPS_PROXY=http://proxy:3128
export HTTP_PROXY=http://proxy:3128
```

## Bootstrapper not sourcing snippets

Run `dots shell check` to verify the bootstrapper is installed. If not:

```
dots shell init
```

Then restart your shell or `source ~/.zshrc`.
