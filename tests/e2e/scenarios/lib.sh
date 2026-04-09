#!/bin/bash
# Shared helpers for e2e scenarios.
set -euo pipefail

PASS=0
FAIL=0

assert_eq() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       expected: %s\n' "$expected"
        printf '       actual:   %s\n' "$actual"
        FAIL=$((FAIL + 1))
    fi
}

assert_contains() {
    local desc="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       expected to contain: %s\n' "$needle"
        printf '       actual: %s\n' "$haystack"
        FAIL=$((FAIL + 1))
    fi
}

assert_file_exists() {
    local desc="$1" path="$2"
    if [ -e "$path" ]; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       file not found: %s\n' "$path"
        FAIL=$((FAIL + 1))
    fi
}

assert_symlink() {
    local desc="$1" path="$2"
    if [ -L "$path" ]; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       not a symlink: %s\n' "$path"
        FAIL=$((FAIL + 1))
    fi
}

assert_not_exists() {
    local desc="$1" path="$2"
    if [ ! -e "$path" ] && [ ! -L "$path" ]; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       file should not exist: %s\n' "$path"
        FAIL=$((FAIL + 1))
    fi
}

assert_file_mode() {
    local desc="$1" expected="$2" path="$3"
    local actual
    actual=$(stat -c '%a' "$path" 2>/dev/null || stat -f '%Lp' "$path" 2>/dev/null)
    if [ "$expected" = "$actual" ]; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       expected mode: %s\n' "$expected"
        printf '       actual mode:   %s\n' "$actual"
        FAIL=$((FAIL + 1))
    fi
}

assert_command_succeeds() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    else
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       command failed: %s\n' "$*"
        FAIL=$((FAIL + 1))
    fi
}

assert_command_fails() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        printf '  \033[31mFAIL\033[0m %s\n' "$desc"
        printf '       command should have failed: %s\n' "$*"
        FAIL=$((FAIL + 1))
    else
        printf '  \033[32mPASS\033[0m %s\n' "$desc"
        PASS=$((PASS + 1))
    fi
}

summary() {
    echo
    local total=$((PASS + FAIL))
    if [ "$FAIL" -eq 0 ]; then
        printf '\033[32m%d/%d passed\033[0m\n' "$PASS" "$total"
    else
        printf '\033[31m%d/%d passed, %d failed\033[0m\n' "$PASS" "$total" "$FAIL"
    fi
    return "$FAIL"
}
