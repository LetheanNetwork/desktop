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
#
# id == the LetheanNetwork repo suffix (LetheanNetwork/sdk-<id>) AND
# the npm-shaped package name (@lthn/sdk-<id>). generator == the
# openapi-generator-cli -g argument; they differ in one place today:
# cpp-qt5 (id) → cpp-qt5-client (generator).
MANIFEST=(
  # ── TypeScript family ──────────────────────────────────────────────
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
  # ── JavaScript family ──────────────────────────────────────────────
  "javascript|javascript|Plain ES6 client without TypeScript."
  "javascript-flowtyped|javascript-flowtyped|JS + Flow type annotations."
  "javascript-closure-angular|javascript-closure-angular|Google Closure Compiler + AngularJS."
  "javascript-apollo|javascript-apollo-deprecated|Apollo GraphQL client (deprecated by upstream — name carries the -deprecated suffix in 7.x)."
  # ── C / C++ ────────────────────────────────────────────────────────
  "c|c|libcurl-based ANSI C client. Embedded, kernel-adjacent, FFI bridge target."
  "cpp-restsdk|cpp-restsdk|Microsoft cpprestsdk (Casablanca). Classic modern-C++ REST client."
  "cpp-qt|cpp-qt-client|Qt client (formerly cpp-qt5-client; renamed to cpp-qt-client in 7.x). QNetworkAccessManager-backed."
  # ── .NET family ────────────────────────────────────────────────────
  "csharp|csharp|Newer C# generator. Multi-target — net8.0 / net6.0 / net47 / net48 / netstandard2.0+."
  # csharp-netcore generator was removed in openapi-generator 7.x —
  # the unified csharp generator handles every .NET target via
  # --additional-properties targetFramework=...
  # ── Apple platforms ────────────────────────────────────────────────
  "objc|objc|Objective-C client (NSURLConnection / AFNetworking). Cocoa / iOS-legacy lane."
  "swift5|swift5|Swift 5+ URLSession client. iOS / macOS / SwiftUI consumers."
  # swift4 generator was removed in openapi-generator 7.x —
  # swift5 is the only Swift client generator.
  # ── Android / JVM / native ─────────────────────────────────────────
  "kotlin|kotlin|Kotlin client for Android + general JVM ecosystems."
  "rust|rust|Rust reqwest-based async client."
  "dart|dart|Dart client (basic HTTP)."
  "dart-dio|dart-dio|Dart with Dio HTTP — Flutter community canon."
  "clojure|clojure|JVM functional Lisp client."
  # ── Scripting / server-side ────────────────────────────────────────
  "php|php|Guzzle-based PHP client. Composer-installable via @lthn/sdk-php."
  "python|python|urllib3-based Python client. pip-installable via @lthn/sdk-python."
  "ruby|ruby|Faraday-based Ruby client. Bundler / gem distribution."
  "java|java|JVM Java client. Maven Central distribution."
  "go|go|Go client (alternative to dappco.re/go for non-dappcore consumers)."
  "powershell|powershell|PowerShell client. Windows admin / scripting consumers."
  "bash|bash|Bash client. Shell-only environments — embedded systems, ops scripts."
  # ── Specialty / extended-reach ─────────────────────────────────────
  "swift-combine|swift-combine|Swift Combine framework flavour — modern reactive iOS / macOS."
  "cpp-ue4|cpp-ue4|Unreal Engine 4 C++ integration. Gaming-side lthn plugins."
  "cpp-tiny|cpp-tiny|Embedded C++ with no STL dependency. Microcontroller-friendly."
  # rust-axum is a SERVER stub, not a client — it scaffolds an Axum
  # service that implements the lthn API surface. Useful for the
  # third-party plugin story ("implement the lthn protocol in Rust"),
  # distinct in shape from sdk-rust which consumes it.
  "rust-axum|rust-axum|Axum server stub — scaffold a Rust service implementing the lthn API surface."
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

  # Per-generator --additional-properties. TS/JS generators consume
  # npmName + npmVersion; other generators have their own knobs
  # (Swift's projectName, Rust's packageName, ObjC's classPrefix, …).
  # We let the native generators use their defaults rather than pass
  # a one-size-fits-all key that they'd warn about and ignore.
  props=""
  case "$gen" in
    typescript*|javascript*)
      props="npmName=$npm_name,npmVersion=0.1.0,supportsES6=true"
      ;;
  esac

  # Global properties — applied to every generator. Forces uniform
  # output across the family:
  #   apiDocs / modelDocs    → per-endpoint + per-model Markdown that
  #                            renders inline on the GitHub repo home
  #   apiTests / modelTests  → test scaffolding the consumer can fill
  #                            in; CI-ready stubs even when the
  #                            generator skips them by default
  #   generateAliasAsModel   → primitive-aliased types (UserId etc.)
  #                            become proper model classes; cleaner
  #                            typing in Swift / Kotlin / Rust
  # See https://openapi-generator.tech/docs/globals for the full list.
  globals="apiDocs=true,modelDocs=true,apiTests=true,modelTests=true,generateAliasAsModel=true"

  rm -rf "$out_dir"
  gen_args=( generate -i "$SPEC_PATH" -g "$gen" -o "$out_dir" --global-property "$globals" )
  [ -n "$props" ] && gen_args+=( --additional-properties "$props" )

  if ! openapi-generator-cli "${gen_args[@]}" >/dev/null 2>&1; then
    echo "✗ openapi-generator-cli failed for $id — skipping"
    failures+=("$id")
    continue
  fi

  # Strip noise files the generators emit that don't belong in a
  # published SDK repo:
  #   git_push.sh         — boilerplate "create a repo + push" helper;
  #                         we own the publish flow.
  find "$out_dir" -maxdepth 2 -name "git_push.sh" -delete 2>/dev/null || true

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
