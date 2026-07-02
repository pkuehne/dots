#!/usr/bin/env bash
# pr-watch.sh — one-shot PR review status for the babysit-PR loop.
#
# Collapses the many gh calls used while shepherding a PR (CI checks,
# Copilot review threads, new review comments) into a single command so it
# can be allow-listed once instead of prompting per compound invocation.
#
# Usage:
#   scripts/pr-watch.sh [PR]            # status snapshot (CI + open threads)
#   scripts/pr-watch.sh [PR] --wait     # block until CI checks finish, then snapshot
#   scripts/pr-watch.sh [PR] --comments # also print full body of open-thread comments
#
# PR defaults to the PR for the current branch.
set -euo pipefail

REPO="${PR_WATCH_REPO:-pkuehne/dots}"

pr=""
wait=false
comments=false
for arg in "$@"; do
  case "$arg" in
    --wait) wait=true ;;
    --comments) comments=true ;;
    *)
      # PR must be strictly numeric — it is interpolated into the GraphQL query.
      if [[ "$arg" =~ ^[0-9]+$ ]]; then
        pr="$arg"
      else
        echo "unknown arg: $arg" >&2
        exit 2
      fi
      ;;
  esac
done

if [[ -z "$pr" ]]; then
  pr=$(gh pr view --repo "$REPO" --json number --jq '.number' 2>/dev/null || true)
  if [[ -z "$pr" ]]; then
    echo "no PR found for the current branch; pass a PR number" >&2
    exit 1
  fi
fi

if $wait; then
  echo "== waiting for CI on PR #$pr =="
  # Right after a push the checks may not be registered yet, and
  # `gh pr checks --watch` exits immediately in that window. Poll until
  # checks appear and settle, re-watching to catch workflows (CodeQL, etc.)
  # that register late. Bounded so a stuck check can't hang forever.
  for _ in $(seq 1 60); do
    status=$(gh pr checks "$pr" --repo "$REPO" 2>/dev/null || true)
    if [[ -z "$status" ]]; then
      sleep 5          # no checks registered yet
      continue
    fi
    if grep -qiE '(pending|queued|in_progress)' <<<"$status"; then
      # --watch blocks until checks settle; it exits non-zero if any check
      # fails, so swallow that rather than aborting the snapshot below.
      gh pr checks "$pr" --repo "$REPO" --watch --fail-fast >/dev/null 2>&1 || true
      sleep 3          # let late-registering workflows show up, then re-check
      continue
    fi
    break              # all checks present and settled
  done
fi

echo "== CI checks (PR #$pr) =="
gh pr checks "$pr" --repo "$REPO" 2>/dev/null || echo "(no checks reported yet)"

echo
echo "== state =="
gh pr view "$pr" --repo "$REPO" --json state,mergeable,reviewDecision \
  --jq '"state=\(.state) mergeable=\(.mergeable) reviewDecision=\(.reviewDecision // "-")"'

echo
echo "== unresolved review threads =="
jq_thread='.data.repository.pullRequest.reviewThreads.nodes[]
  | select(.isResolved==false)
  | "- [\(.path):\(.comments.nodes[-1].line // "?")] last=\(.comments.nodes[-1].author.login) @ \(.comments.nodes[-1].createdAt)"'
owner="${REPO%%/*}"
name="${REPO##*/}"
threads=$(gh api graphql -f query="
{ repository(owner:\"$owner\", name:\"$name\") {
    pullRequest(number: $pr) {
      reviewThreads(last: 30) {
        nodes { isResolved path
          comments(last: 1) { nodes { author { login } createdAt line body } } } } } } }" \
  --jq "$jq_thread" 2>/dev/null || true)

if [[ -z "$threads" ]]; then
  echo "(none — all threads resolved)"
else
  echo "$threads"
  if $comments; then
    echo
    echo "== open-thread comment bodies =="
    gh api graphql -f query="
    { repository(owner:\"$owner\", name:\"$name\") {
        pullRequest(number: $pr) {
          reviewThreads(last: 30) {
            nodes { isResolved path
              comments(last: 1) { nodes { author { login } body } } } } } } }" \
      --jq '.data.repository.pullRequest.reviewThreads.nodes[]
        | select(.isResolved==false)
        | "--- \(.path) (\(.comments.nodes[-1].author.login)) ---\n\(.comments.nodes[-1].body)\n"'
  fi
fi
