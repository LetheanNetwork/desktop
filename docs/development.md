---
title: Development Guide
description: Build, run, test, and extend the Lethean Desktop CLI, Wails host, and Angular frontend.
---

<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Development Guide

Lethean Desktop builds the `lthn` CLI router and the native/browser hosts that
consume its Go services. The Angular application is the only product
frontend. Wails owns native host behaviour and transport; Angular owns
rendering and navigation.

Read [`../AGENTS.md`](../AGENTS.md) before changing architecture, CoreGO
composition, file access, or frontend boundaries.

## 1. Toolchain and checkout

Use the versions declared by:

- `go/go.mod`
- `frontend-ng/package.json`
- `frontend-ng/package-lock.json`
- `.github/workflows/build.yml`

Required local commands are Go, Node.js, npm, Wails 3, and Task. A normal clone
contains the complete source topology:

```bash
git clone <repo-url> lthn-desktop
cd lthn-desktop
go work sync
cd frontend-ng
npm ci
cd ..
wails3 task doctor
```

There are no required Git submodules or `external/` source checkouts.
`go.work` contains only `./go`; CoreGO dependencies resolve from the versioned
`dappco.re/go*` modules in `go/go.mod`.

`task doctor` distinguishes required failures from optional resources. Missing
crew repositories, generated bindings, or occupied development ports are
reported with direct remedies. Optional sibling repositories can be selected
with:

```text
LTHN_MLX_REPO
LTHN_AGENT_REPO
LTHN_AI_REPO
```

## 2. Development modes

### Browser-only UI and demo development

```bash
cd frontend-ng
npm run demo
```

Open:

```text
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=desktop#/
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=shell#/
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=device&lthn-device=small#/
```

`lthn-offline=1` deliberately disables Wails socket retries and event
subscriptions. Demo data remains visibly labelled and isolated per app
window, so UI work is deterministic without a native backend.

### Full Wails development

From the repository root:

```bash
wails3 task dev
```

The development topology is:

| Port | Owner | Purpose |
|---|---|---|
| `9245` | Angular development server | HMR and development assets |
| `9099` | Wails MCP service | Development-only WebView automation |
| `9199` | Lethean connection service | Generated binding calls and events |

On macOS the development window loads the Angular loopback URL directly.
WebKit rejects JavaScript WebSockets opened from the secure custom
`wails://` scheme, so the host injects the exact validated `lthn-ws` URL into
the development query. Production does not use this route: it loads embedded
assets through `wails://`.

The Files app opens existing `Documents` and `Downloads` roots through
sandboxed `io.Medium` providers. macOS prompts for those protected folders
using the descriptions in `build/darwin/Info.dev.plist` and
`build/darwin/Info.plist`. If access was previously denied, enable Lethean
Desktop under **System Settings → Privacy & Security → Files & Folders**.
Denial deliberately leaves the provider unavailable; it never falls back to
raw host paths.

Angular changes update through HMR. Go changes regenerate TypeScript bindings,
compile the host-native development binary, and relaunch Wails without
rebuilding the production frontend or optional crew sidecars.

If a previous session owns a port, close the running application before
starting another session or running focused desktop tests.

## 3. Builds and packages

Build only the Angular production bundle:

```bash
cd frontend-ng
npm run build
```

Output is written directly to:

```text
go/cmd/lthn/dist/index.html
```

`go/cmd/lthn/embed.go` embeds that directory into production builds.

Build or package the current platform:

```bash
wails3 task build
wails3 task package
```

Platform-specific tasks live under:

```text
build/darwin/
build/linux/
build/windows/
build/ios/
build/android/
```

The root production pre-build may stage `lthn-mlx`, `lthn-agent`, and
`lthn-ai`. Their source repositories are optional; absence must not prevent a
GUI-only build.

Build the CLI router directly when no native package is needed:

```bash
go build -o bin/lthn ./go/cmd/lthn
```

The CLI remains usable independently of the GUI:

```bash
./bin/lthn help
./bin/lthn version
./bin/lthn serve
./bin/lthn ai
```

## 4. Generated Wails bindings

Generate the desktop bindings with:

```bash
wails3 task common:generate:bindings
```

Every platform generator writes to the shared ignored directory:

```text
frontend-ng/bindings/
```

Desktop, iOS, and Android tasks use flavour markers so one platform cannot
silently reuse another platform's bindings. Mobile public binding tasks
restore the desktop flavour afterwards. Generation uses `go work sync`; it
does not recreate the removed submodule workspace.

When adding a bindable service or method, regenerate bindings and run the
frontend contract suite before relying on generated TypeScript.

## 5. Tests and confidence gates

Focused iteration:

```bash
go test ./go/pkg/<changed-package>

cd frontend-ng
npx ng test --watch=false --include=src/path/to/file.spec.ts
```

Repository entrypoints:

```bash
wails3 task test:go
wails3 task test:frontend
wails3 task test

wails3 task test:cover:go
wails3 task test:cover:frontend
wails3 task test:cover
```

The ordered frontend confidence gate used by CI is:

```bash
wails3 task verify:frontend
```

It runs formatting, type/build checks, Angular tests, executable development
contracts, capability inventory, production build, and output verification in
one deterministic order.

Before stopping after code changes, run checks proportional to the scope:

```bash
gofmt -l go/
git diff --check
go vet ./go/...
wails3 task test
cd frontend-ng && npm run build
```

The repository has known historical formatting, coverage, and CoreGO
compliance debt. Report broad diagnostics honestly; do not rewrite unrelated
packages or weaken tests to manufacture an all-green global claim.

## 6. Audit entrypoint

Run:

```bash
bash build/audit.sh
```

Use `-v` for full captured output:

```bash
bash build/audit.sh -v
```

The script treats Go vet/build/test and frontend build/test/contract failures
as blocking. The external CoreGO v0.9.0 audit is a no-regression diagnostic:
its large pre-existing compliance backlog is reported but is not an all-zero
gate for unrelated changes. If the external audit checkout is absent, the
script records that fact and continues with repository-owned gates.

## 7. Adding a CLI command

CLI verbs are flat handlers in `go/cmd/lthn/main.go`:

```go
func cmdNewcmd(args []string) int {
	// Parse the verb's arguments, delegate to go/pkg capability, and return
	// 0 for success, 1 for runtime failure, or 2 for usage failure.
	return 0
}
```

Register the dispatch branch and help text in the same file. Reusable
capability belongs in `go/pkg/*`, never in `cmd/lthn`.

Use existing CoreGO process boundaries:

- `core.Args`, `core.Exit`
- `core.Print`, `core.Println`
- `core.ParseFlag`
- `core.Result`, `core.Ok`, `core.Fail`

Add focused Good/Bad/Ugly tests and a runnable example for new public
behaviour.

## 8. Adding a Go service

A canonical service exposes:

```go
type Options struct {
	// Explicit dependencies.
}

type Service struct {
	// Owned state.
}

func NewService(options Options) *Service {
	return &Service{}
}

func (service *Service) Register(coreInstance *core.Core) core.Result {
	return core.Ok(service)
}

func Register(coreInstance *core.Core) core.Result {
	return NewService(Options{}).Register(coreInstance)
}
```

Include a `// Usage example:` marker and package examples. Register lifecycle
and service wiring through `core.Core`; do not hide process-wide composition
in package globals.

All file-backed product operations must ultimately pass through a registered
`dappco.re/go/io.Medium`. Renderer calls carry mount IDs and
provider-relative paths, never absolute host paths. A missing Medium fails
closed.

Tests which need user-data roots must use `t.TempDir()` and an isolated
`HOME`. Never write into the developer's real `~/Lethean/` tree.

## 9. Adding an Angular surface

The canonical registries are:

- `frontend-ng/src/app/desktop/desktop-catalogue.data.ts`
- `frontend-ng/src/app/desktop/surfaces/surface-registry.ts`
- `frontend-ng/src/app/desktop/apps/app-view.ts`
- `frontend-ng/src/app/desktop/desktop-route-tree.ts`

Add the application/category metadata and lazy standalone component, then let
the route tree derive navigation. Extend route/registry tests rather than
adding a parallel view switcher.

Prefer:

- standalone components;
- `ChangeDetectionStrategy.OnPush`;
- signals for component-local reactive state;
- NgRx for shared or transport-driven state; and
- typed bridge services which validate unknown native payloads.

Lit remains intentional only for reusable custom elements under
`frontend-ng/src/kit/` and plugin descriptors whose `kind` is `lit`. Do not
restore the retired Lit application.

## 10. Connection configuration

The generic connection service defaults to
`ws://localhost:9099/wails/ws`. Full Wails development moves the Lethean
transport to 9199 because the development-only Wails MCP service owns 9099.

| Environment variable | Purpose |
|---|---|
| `LTHN_WAILS_WS_LISTEN` | Backend listen address |
| `LTHN_WAILS_WS_PATH` | HTTP WebSocket upgrade path |
| `LTHN_WAILS_WS_URL` | Public WebSocket URL or root-relative proxy path |
| `LTHN_WAILS_WS_ORIGINS` | Comma-separated exact browser origins |
| `LTHN_WAILS_WS_TOKEN` | Optional upgrade token |
| `LTHN_WAILS_WS_TRUST_PROXY` | Explicit non-loopback proxy acknowledgement |

Loopback is the safe default. Remote browser clients require WSS plus an exact
origin policy and authentication. Never place long-lived tokens in URLs,
browser history, generated JavaScript, logs, or persistent mobile storage.

## 11. Coding standards

- Use British English in code, copy, tests, and docs.
- Use EUPL-1.2 identifiers and headers.
- Treat manifests as dependency-version truth; avoid copied patch-version
  claims in prose.
- Keep the CLI usable when GUI assets or hosts fail.
- Preserve hash routing and client-side rendering.
- Do not add SSR, hydration, a second product frontend, feature paywalls, or
  “Pro” gates.
- Use TDD for behavioural changes.
- Preserve unrelated work in dirty worktrees.
