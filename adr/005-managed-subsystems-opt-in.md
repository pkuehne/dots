# ADR 005: Managed Subsystems are Opt-in

## Context
dots can manage shell init, git config, and SSH config. Users should not be forced to delegate control of these.

## Decision
Each managed subsystem requires `managed = true`. Users own what they don't delegate. Enabling one does not affect others.

## Consequences
- Unlike Home Manager, not everything must be declared before anything works
- Incremental adoption: start with file mirroring, add subsystems as needed
- Trade-off: less compositional; user must manually opt in per subsystem

## Alternatives Considered
- All-or-nothing management: too aggressive for a dotfile tool
- Auto-detection of what to manage: unpredictable, hard to debug
