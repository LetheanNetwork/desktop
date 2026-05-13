#!/usr/bin/env bash
# Publish all @lthn/sdk-* flavours from a fresh OpenAPI spec.
#
# For each entry in MANIFEST: regenerate the SDK with
# openapi-generator-cli, drop a README + package.json overlay
# (preserving generator-emitted fields where present), init the
# directory as a git repo, and force-push to LetheanNetwork/sdk-<id>.
#
# The matching empty repos must already exist on GitHub. Auth uses
# the caller's git credential helper (HTTPS) — same as `git push`
# from any other Lethean repo.
#
# Usage:
#
#   build/sdk/publish.sh              # publish all flavours
#   build/sdk/publish.sh <id>...      # publish a subset
#
# Requirements:
#   - openapi-generator-cli on PATH (`npm i -g @openapitools/openapi-generator-cli`)
#   - Java JDK on PATH (`brew install openjdk` on macOS)
#   - `lthn` binary built (we'll call `go run ./cmd/lthn api spec`)
#   - GitHub push access to LetheanNetwork/sdk-*

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SDK_DIR="$REPO_ROOT/build/sdk"
SPEC_PATH="$SDK_DIR/openapi.yaml"
DESC="lthn HTTP gateway SDK — auto-generated from the live api.Engine. See https://github.com/LetheanNetwork/desktop for source."

# id|generator|description-suffix. apollo is excluded — its
# openapi-generator-cli 7.22.0 backend currently errors out before
# emitting files; revisit when upstream fixes it.
MANIFEST=(
  "typescript-fetch|typescript-fetch|Zero-dependency fetch() client. Default for Lit / Wails WebView / vanilla TS apps."
  "typescript-axios|typescript-axios|axios client — Node + browser, the de-facto ergonomic REST shape."
  "typescript|typescript|Configurable backend (fetch / axios / jquery) selected via --additional-properties."
  "typescript-angular|typescript-angular|Angular HttpClient + RxJS service classes, DI-ready."
  "typescript-rxjs|typescript-rxjs|RxJS observables, framework-agnostic. Use outside Angular for reactive pipelines."
  "typescript-node|typescript-node|Node-only client. Callbacks + promises. CLI scripts and server agents."
  "typescript-redux-query|typescript-redux-query|redux-query actions + reducers."
  "typescript-inversify|typescript-inversify|InversifyJS DI containers."
  "typescript-aurelia|typescript-aurelia|Aurelia framework client."
  "typescript-jquery|typescript-jquery|jQuery \$.ajax client — legacy support."
  "javascript|javascript|Plain ES6 client without TypeScript."
  "javascript-flowtyped|javascript-flowtyped|JS + Flow type annotations."
  "javascript-closure-angular|javascript-closure-angular|Google Closure Compiler + AngularJS."
)

# Filter to selected ids if any were passed as args.
select_ids=("$@")
should_skip() {
  local id="$1"
  if [ ${#select_ids[@]} -eq 0 ]; then
    return 1
  fi
  for s in "${select_ids[@]}"; do
    [ "$s" = "$id" ] && return 1
  done
  return 0
}

# Make sure the spec is fresh — regenerate from the live engine.
echo "→ Regenerating $SPEC_PATH from the live api.Engine"
mkdir -p "$SDK_DIR"
( cd "$REPO_ROOT/go" && go run ./cmd/lthn api spec --format yaml --out "$SPEC_PATH" )

failures=()
successes=()
for row in "${MANIFEST[@]}"; do
  IFS='|' read -r id gen desc <<<"$row"
  if should_skip "$id"; then continue; fi

  out_dir="$SDK_DIR/$id"
  remote_url="https://github.com/LetheanNetwork/sdk-$id.git"
  npm_name="@lthn/sdk-$id"

  echo ""
  echo "═══════════════════════════════════════════════════════════════"
  echo "  $id  →  $remote_url"
  echo "═══════════════════════════════════════════════════════════════"

  rm -rf "$out_dir"
  if ! openapi-generator-cli generate \
      -i "$SPEC_PATH" \
      -g "$gen" \
      -o "$out_dir" \
      --additional-properties "npmName=$npm_name,npmVersion=0.1.0,supportsES6=true" \
      >/dev/null 2>&1; then
    echo "✗ openapi-generator-cli failed for $id — skipping"
    failures+=("$id")
    continue
  fi

  # README — present on every flavour for consistent landing experience.
  cat > "$out_dir/README.md" <<README
# $npm_name

$desc

Auto-generated from the lthn OpenAPI spec at:

  https://github.com/LetheanNetwork/desktop/blob/main/build/sdk/openapi.yaml

Do **not** hand-edit. Regenerate via:

  \`\`\`bash
  cd /path/to/desktop
  ./build/sdk/publish.sh $id
  \`\`\`

## Install

\`\`\`bash
npm install $npm_name
\`\`\`

## License

EUPL-1.2 — same as the upstream lthn project.
README

  # package.json — generator-emitted on most flavours, missing on
  # typescript-fetch. Drop a minimal one when absent.
  if [ ! -f "$out_dir/package.json" ]; then
    cat > "$out_dir/package.json" <<PKG
{
  "name": "$npm_name",
  "version": "0.1.0",
  "description": "$DESC",
  "license": "EUPL-1.2",
  "main": "./dist/index.js",
  "module": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/LetheanNetwork/sdk-$id.git"
  },
  "publishConfig": {
    "access": "public"
  }
}
PKG
  fi

  ( cd "$out_dir" && \
    git init --quiet --initial-branch=main && \
    git remote add origin "$remote_url" && \
    git add -A && \
    git -c user.email=virgil@lethean.io -c user.name="Virgil" \
        commit --quiet -m "feat: initial @lthn/sdk-$id from openapi.yaml

Generated from build/sdk/openapi.yaml via openapi-generator-cli
($gen). See parent repo for source: https://github.com/LetheanNetwork/desktop.

Co-Authored-By: Virgil <virgil@lethean.io>" && \
    git push --force --quiet origin main ) && {
      echo "✓ pushed $id"
      successes+=("$id")
    } || {
      echo "✗ push failed for $id"
      failures+=("$id")
    }
done

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  Summary"
echo "═══════════════════════════════════════════════════════════════"
echo "  ${#successes[@]} succeeded: ${successes[*]:-(none)}"
echo "  ${#failures[@]} failed:    ${failures[*]:-(none)}"
[ ${#failures[@]} -eq 0 ]
