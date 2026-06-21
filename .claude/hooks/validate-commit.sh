#!/usr/bin/env bash
# PreToolUse hook: enforce Conventional Commits on every git commit Claude makes.
# Receives a JSON payload on stdin; exits 2 to block, 0 to allow.

input=$(cat)

cmd=$(printf '%s' "$input" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('tool_input', {}).get('command', ''))
" 2>/dev/null)

# Only inspect git commit commands
echo "$cmd" | grep -qE '\bgit commit\b' || exit 0

# Allow --amend --no-edit (no new message to validate)
echo "$cmd" | grep -qE 'git commit.*(--amend[[:space:]].*--no-edit|--no-edit[[:space:]].*--amend)' && exit 0

# --- Extract the full commit message ---

full_msg=""

# 1. Heredoc pattern (Claude's standard form):
#    git commit -m "$(cat <<'EOF'
#       subject
#
#       body
#       Co-Authored-By: ...
#       EOF
#    )"
if echo "$cmd" | grep -q 'EOF'; then
    full_msg=$(printf '%s' "$cmd" \
        | sed -n "/<<[[:space:]]*'\\?EOF'\\?/,/^[[:space:]]*EOF[[:space:]]*\$/p" \
        | grep -v 'EOF' \
        | sed 's/^[[:space:]]*//')
fi

# 2. Inline -m "..." or --message "..."
if [ -z "$full_msg" ]; then
    full_msg=$(printf '%s' "$cmd" \
        | grep -oP '(?<=-m ")[^"]+|(?<=--message ")[^"]+' \
        | head -1 \
        | sed 's/^[[:space:]]*//')
fi

# Can't determine message — let git handle it
[ -z "$full_msg" ] && exit 0

# --- Validate subject (first non-empty line) ---

subject=$(printf '%s' "$full_msg" | grep -m1 '.')

if [ -z "$subject" ]; then
    echo "Commit blocked: commit message is empty."
    exit 2
fi

# Conventional Commits: type(scope)!: subject
cc_re='^(feat|fix|chore|docs|test|refactor|style|ci|perf|build|revert)(\([a-z0-9,/_-]+\))?(!)?: .+'
if ! printf '%s' "$subject" | grep -qE "$cc_re"; then
    cat <<'EOF'
Commit blocked: subject does not follow Conventional Commits format.

  Required format:  <type>(<scope>): <subject>

  type  : feat | fix | refactor | test | docs | chore | ci | perf | revert
  scope : optional, e.g. config, tools, deploy, shell, git, ssh
  !     : optional — marks a breaking change

  Examples:
    feat(config): add SSH managed mode
    fix(tools): handle stale symlinks on re-apply
    chore!: drop Python 3.9 support

See .gitmessage for the full template.
EOF
    exit 2
fi

len=${#subject}
if [ "$len" -gt 70 ]; then
    echo "Commit blocked: subject is $len chars (max 70)."
    printf '  %s\n' "$subject"
    exit 2
fi

# --- Validate structure: blank line then body ---

# Second line must be blank when there are more lines
second=$(printf '%s' "$full_msg" | sed -n '2p')
if [ -n "$second" ] && [ -n "$(printf '%s' "$second" | tr -d '[:space:]')" ]; then
    echo "Commit blocked: second line must be blank (separate subject from body)."
    exit 2
fi

# Body is required: at least one non-blank, non-trailer line after the subject
# Trailers (git footers) do not count as body content
trailers_re='^(Co-Authored-By|BREAKING CHANGE|Closes|Fixes|Refs|Signed-off-by|Reviewed-by):'
body=$(printf '%s' "$full_msg" \
    | awk 'NR > 1' \
    | sed '/^[[:space:]]*$/d' \
    | grep -vE "$trailers_re")

if [ -z "$body" ]; then
    cat <<'EOF'
Commit blocked: a body is required.

  After a blank line, explain WHAT changed and WHY this approach was chosen.
  Call out any decisions, trade-offs, or alternatives considered — especially
  if there were open questions or back-and-forth during implementation.

  Format:
    <type>(<scope>): <subject>

    What changed and why. Note any design decisions or trade-offs
    that shaped this implementation.

See .gitmessage for the full template.
EOF
    exit 2
fi

exit 0
