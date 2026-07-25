#!/usr/bin/env bash
# lthn-desktop audit — run this before/after every commit.
#
# Usage:
#   bash build/audit.sh           # quiet — failures + compliance diagnostic
#   bash build/audit.sh -v        # verbose — full output of every step
#
# Two passes: the current compliance backlog is diagnostic, while build and
# test failures remain blocking:
#
#   1. v0.9.0 compliance — canonical no-regression diagnostic; reference
#      script lives in core/go's test tree.
#   2. Build + test — go vet, go build, go test, frontend build,
#      frontend test, frontend convergence contracts.
#
# Quiet mode (default) prints one line per step (✓ or ✗) and only
# the failing step's captured stdout/stderr on a ✗. For v0.9.0 it
# surfaces just the verdict + any non-zero counters. Cuts ~1.2k lines
# of green-path noise down to <30 lines.
set -eu

cd "$(dirname "$0")/.."

V090_AUDIT="/Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh"

VERBOSE=0
[ "${1:-}" = "-v" ] && VERBOSE=1

# step <name> <command...> — runs the command, captures output, prints
# a one-liner; on failure prints the captured output and exits 1.
step() {
  local name="$1"
  shift
  local out
  if out=$("$@" 2>&1); then
    if [ "$VERBOSE" = "1" ]; then
      printf '\n== %s ==\n%s\n' "$name" "$out"
    fi
    printf '✓ %s\n' "$name"
    return 0
  fi
  printf '✗ %s\n' "$name"
  echo "$out"
  exit 1
}

# Only the verdict + any non-zero counters surface in quiet mode. Verbose mode
# retains the full diagnostic backlog.
audit_v090() {
  if [ ! -f "$V090_AUDIT" ]; then
    printf '⚠ v0.9.0 compliance — script missing at %s (skipped)\n' "$V090_AUDIT"
    return 0
  fi
  local out
  out=$(bash "$V090_AUDIT" . 2>&1 || true)
  if echo "$out" | grep -q 'verdict: .*COMPLIANT[^—]*$'; then
    printf '✓ v0.9.0 compliance — COMPLIANT\n'
    [ "$VERBOSE" = "1" ] && echo "$out"
    return 0
  fi
  printf '⚠ v0.9.0 compliance — non-compliant baseline (diagnostic)\n'
  if [ "$VERBOSE" = "1" ]; then
    echo "$out"
  else
    # Surface only non-zero counters and the verdict line; drop the
    # green "0    (description)" rows so only real findings show.
    echo "$out" | grep -E '^\s+[a-z][a-z-]+\s+[1-9]|verdict:' || true
  fi
  return 0
}

audit_v090

step "go vet"            go vet ./go/...
step "go build"          bash -c 'go build -o /tmp/lthn-audit ./go/cmd/lthn && rm -f /tmp/lthn-audit'
step "go test"           go test -count=1 ./go/...
step "frontend build"    bash -c 'cd frontend-ng && npm run build'
step "frontend test"     bash -c 'cd frontend-ng && npm run test:ci'
step "frontend contracts" bash -c 'cd frontend-ng && npm run test:contracts'

echo
echo PASS
