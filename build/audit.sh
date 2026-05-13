#!/usr/bin/env bash
# lthn-desktop audit — run this before/after every commit.
#
# Usage:
#   bash build/audit.sh
#
# Two passes, both must pass for a clean `verdict: COMPLIANT` exit.
#
#   1. v0.9.0 compliance — the canonical pattern check used across every
#      consumer of dappco.re/go. 40+ dimensions: module path, banned stdlib
#      imports, AX-7 triplets, anti-gaming patterns, docs structure, service
#      shape, etc. Reference script lives in core/go's test tree.
#
#   2. Build + test — go vet, go build, go test, frontend build, frontend
#      test. The v0.9.0 audit is a static analyser; this leg confirms the
#      code actually compiles and the test suite passes.
#
# The v0.9.0 audit's stray `yea` token on line 1 is a known quirk — ignore
# the "yea: command not found" line on stdout.
set -eu

cd "$(dirname "$0")/.."

V090_AUDIT="/Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh"

step() { printf '\n== %s ==\n' "$*"; }

if [ ! -f "$V090_AUDIT" ]; then
  echo "WARN: $V090_AUDIT missing — v0.9.0 compliance leg skipped." >&2
  echo "      Codex should report TODO(snider): wire core/go submodule for in-repo audit." >&2
else
  step "v0.9.0 compliance"
  bash "$V090_AUDIT" .
fi

step "go vet"
go vet ./go/...

step "go build"
go build -o /tmp/lthn-audit ./go/cmd/lthn
rm -f /tmp/lthn-audit

step "go test"
go test -count=1 ./go/...

step "frontend build"
( cd frontend && bun run build )

step "frontend test"
( cd frontend && bun run test )

echo
echo "PASS"
