#!/bin/bash
# Run all e2e scenarios inside Docker containers.
# Usage: ./tests/e2e/run.sh [scenario-number...]
#   ./tests/e2e/run.sh           # run all
#   ./tests/e2e/run.sh 01 08     # run specific scenarios
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

IMAGE="dots-e2e"
IMAGE_TERMUX="dots-e2e-termux"
SCENARIOS_DIR="tests/e2e/scenarios"
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_SCENARIOS=""

# ── build images ──
printf '\033[1m==> Building Linux image\033[0m\n'
docker build -t "$IMAGE" -f tests/e2e/Dockerfile . --quiet

printf '\033[1m==> Building Termux image\033[0m\n'
docker build -t "$IMAGE_TERMUX" -f tests/e2e/Dockerfile.termux . --quiet

echo

run_scenario() {
    local script="$1"
    local name
    name=$(basename "$script" .sh)
    local image="$IMAGE"

    # Use termux image for termux scenario
    if [[ "$name" == *termux* ]]; then
        image="$IMAGE_TERMUX"
    fi

    printf '\033[1m--- %s ---\033[0m\n' "$name"

    local output exit_code
    output=$(docker run --rm "$image" -c "bash ~/scenarios/$name.sh" 2>&1) && exit_code=0 || exit_code=$?

    echo "$output"

    if [ "$exit_code" -eq 0 ]; then
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
        FAILED_SCENARIOS="$FAILED_SCENARIOS $name"
    fi
    echo
}

# ── select scenarios ──
if [ $# -gt 0 ]; then
    for num in "$@"; do
        script=$(ls "$SCENARIOS_DIR"/${num}-*.sh 2>/dev/null | head -1)
        if [ -z "$script" ]; then
            printf '\033[31mNo scenario matching: %s\033[0m\n' "$num"
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
            continue
        fi
        run_scenario "$script"
    done
else
    for script in "$SCENARIOS_DIR"/[0-9]*.sh; do
        run_scenario "$script"
    done
fi

# ── summary ──
echo "================================="
total=$((TOTAL_PASS + TOTAL_FAIL))
if [ "$TOTAL_FAIL" -eq 0 ]; then
    printf '\033[1;32mAll %d scenarios passed\033[0m\n' "$total"
else
    printf '\033[1;31m%d/%d scenarios passed, %d failed:%s\033[0m\n' \
        "$TOTAL_PASS" "$total" "$TOTAL_FAIL" "$FAILED_SCENARIOS"
fi

exit "$TOTAL_FAIL"
