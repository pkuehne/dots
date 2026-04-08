# ADR 009: Preset Eject Pattern

## Context
Presets provide opinionated defaults (fzf config, tmux config). Users should be able to start with a preset and customize freely.

## Decision
Presets can be ejected: `dots presets eject PRESET` writes the generated output to plain files and sets the preset flag to false. After ejection, the user owns the file.

## Consequences
- No lock-in: presets are a starting point, not a permanent dependency
- Trade-off: ejected config no longer benefits from future preset improvements
- Clear separation between "generated" and "owned" states

## Alternatives Considered
- Non-ejectable presets: creates lock-in
- Template-only approach: harder to get started with
