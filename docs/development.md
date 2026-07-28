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
- `frontend/package.json`
- `frontend/package-lock.json`
- `.github/workflows/build.yml`

Required local commands are Go, Node.js, npm, and Task. Wails is invoked
through the tool directive in `go/go.mod`, so a normal clone contains the
complete source topology:

```bash
git clone <repo-url> lthn-desktop
cd lthn-desktop
go work sync
cd frontend
npm ci
cd ..
go tool wails3 task doctor
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
LTHN_LEM_REPO
LTHN_LEM_BIN
```

## 2. Development modes

### Browser-only UI and demo development

```bash
cd frontend
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
go tool wails3 task dev
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

The native product floor is macOS 26.0. Root Task entrypoints apply the
matching cgo compile and link flags, and both application plists declare the
same minimum. Prefer those Task entrypoints for linked Go builds and tests;
an unwrapped `go build` or `go test` inherits Go 1.26's older macOS link
default unless the same cgo flags are supplied.

The Files app opens existing `Documents` and `Downloads` roots through
sandboxed `io.Medium` providers. macOS prompts for those protected folders
using the descriptions in `build/darwin/Info.dev.plist` and
`build/darwin/Info.plist`. If access was previously denied, enable Lethean
Desktop under **System Settings → Privacy & Security → Files & Folders**.
Denial deliberately leaves the provider unavailable; it never falls back to
raw host paths.

### Desktop state and Settings

Connected structured UI state is owned by `go/pkg/desktopstate` and written
through the registered application `io.Medium`:

```text
desktop/state/shell-session.json
desktop/state/terminal-workspace.json
```

The versioned documents use bounded payloads, optimistic revisions, and a
verified staged/backup commit. A missing Medium, malformed document, revision
conflict, or failed recovery fails closed. The shell document contains only
the current view/device, focus and z-order, and catalogue-derived window
identity, route, grouping, geometry, minimise/maximise state. Native outer
windows, Files authority, commands, credentials, and process state do not
belong there.

Scalar Settings remain in `go/pkg/appconfig`. The Settings surface edits an
immutable draft: **Apply** validates the complete change set, commits it once,
and only then applies live CoreGUI changes; **Discard** restores the committed
snapshot; **Reset** prepares defaults as a draft. A failed commit preserves
the previous committed/live state and reports the failure. Restart-required
controls remain visibly identified.

Application permission policy and verified host permission state are separate
values in Settings. Status reads never prompt. A native request is made only
after the user chooses **Request host access** for a capability which the host
can actually request; unsupported hosts remain visibly unsupported.

In explicit offline mode the shell session, Terminal workspace, Settings
snapshot, and permission status use isolated in-memory providers. They are not
written to the application Medium and they make no Wails call or event
subscription.

### Terminal workspace and reconnect behaviour

`go/pkg/terminal` owns transient PTYs and bounded scrollback. Output chunks
carry `start`/`end` cursors plus a `reset` flag. The Angular terminal retains
its visible xterm buffer while transport reconnects, reattaches after its last
accepted cursor, ignores duplicates, and requests recovery when it sees a gap.
If the retained ring has moved past that cursor, the host sends one bounded
reset snapshot. If the in-memory session no longer exists, the tab becomes
visibly exited and offers an explicit fresh shell.

The Terminal workspace document persists only tab order, active key, title,
shell/agent kind, and a repository or Files mount plus provider-relative path.
It never persists input, output, command arrays, environment, absolute paths,
PIDs, transient session IDs, tokens, or credentials. On a later application
run, an ordinary tab opens a fresh shell in its authorised workspace when the
Terminal surface is opened. A shared-agent tab reattaches only when the
trusted Terminal `List` result still reports that session; otherwise it stays
exited.

Browser demo Terminal remains deliberately non-executing and read-only. It
starts no local process and installs no Wails listener.

### Native launch and host intents

macOS, Linux, Windows NSIS, and Windows MSIX use one product identity,
executable, `lthn://` scheme, and `.lthn`/`application/x-lethean` association.
Launching the CLI with one bounded associated URL or document starts the
Wails host; ordinary CLI verbs retain their existing routing.

Native opens, drops, notification responses, permission snapshots, tray
targets, and deep links converge on the typed `lthn:host:intent` boundary.
Trusted Go converts host paths into least-authority Files capabilities before
emission. Angular validates the exact bounded envelope and routes only through
the desktop catalogue. `.lthn` items open Settings import review; other
supported items open Files. Unknown notifications, actions, permissions, and
routes are ignored or reported unavailable without widening authority.

After changing platform metadata or native event handling, run:

```bash
node --test scripts/verify-native-integration.test.mjs
```

Angular changes update through HMR. Go changes regenerate TypeScript bindings,
compile the host-native development binary, and relaunch Wails without
rebuilding the production frontend or optional crew sidecars.

If a previous session owns a port, close the running application before
starting another session or running focused desktop tests.

### Manual LEM model runtime

LEM is an optional fixed sibling process, not an automatically started
dependency. Control starts it only after an explicit user action, through the
central managed-services system and `go-process`. A start is deliberately
model-less:

```text
lem serve --addr 127.0.0.1:36911 --shutdown-timeout 10s
```

The renderer can then select a model by opaque `model-…` ID. Catalogue
discovery is limited to `~/Lethean/lem/models` through the registered
application `io.Medium`; native model paths never cross Wails. The Go runtime
reads LEM's bounded admin credential through the same Medium at
`lem/admin.token`. The token, endpoint, command, arguments, environment, and
working directory remain trusted-Go details.

The lifecycle is explicit: **Start**, **Load**, **Unload**, **Restart**, and
**Stop**. Closing a window does not stop a running service, while application
shutdown does. Registration, catalogue reads, subscriptions, and refreshes
never start LEM or load a model.

Browser demo mode simulates that lifecycle entirely in memory. It makes no
Wails call or event subscription, which keeps Control and Telemetry available
for deterministic UI work without a model or backend.

## 3. Builds and packages

Build only the Angular production bundle:

```bash
cd frontend
npm run build
```

Output is written directly to:

```text
go/cmd/lthn/dist/index.html
```

`go/cmd/lthn/embed.go` embeds that directory into production builds.

Build or package the current platform:

```bash
go tool wails3 task build
go tool wails3 task package
```

Platform-specific tasks live under:

```text
build/darwin/
build/linux/
build/windows/
build/ios/
build/android/
```

The root production pre-build may stage LEM, `lthn-mlx`, `lthn-agent`, and
`lthn-ai`. Their source repositories are optional; absence must not prevent a
GUI-only build. LEM is built from `LTHN_LEM_REPO` with go-inference's
`build:embed` task on macOS or `build:native` on Linux/Windows. CI can provide
a matching-platform binary with `LTHN_LEM_BIN`. The macOS application bundle,
Linux AppImage/nFPM packages, and default Windows NSIS installer place it
beside `lthn` as `lem` or `lem.exe`; runtime discovery never falls back to
`PATH`.

To stage only the current-platform LEM sidecar:

```bash
go tool wails3 task build:lem
```

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
go tool wails3 task common:generate:bindings
```

Every platform generator writes to the shared ignored directory:

```text
frontend/bindings/
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

cd frontend
npx ng test --watch=false --include=src/path/to/file.spec.ts
```

Focused desktop-state, Terminal, Settings, and native-intent gate:

```bash
go test ./go/pkg/desktopstate ./go/pkg/appconfig ./go/pkg/terminal \
  ./go/pkg/permissions ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/desktopstate ./go/pkg/appconfig ./go/pkg/terminal \
  ./go/pkg/permissions ./go/pkg/desktop ./go/cmd/lthn
node --test scripts/verify-frontend-convergence.test.mjs \
  scripts/verify-native-integration.test.mjs

cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-state-bridge.service.spec.ts \
  --include=src/app/desktop/terminal-workspace.service.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal-session.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal.spec.ts \
  --include=src/app/desktop/desktop-host-intent.service.spec.ts \
  --include=src/app/desktop/desktop-permissions-bridge.service.spec.ts \
  --include=src/app/desktop/apps/settings.app.spec.ts
```

Focused model-runtime confidence gate:

```bash
node --test scripts/verify-model-runtime-convergence.test.mjs
go test ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/services ./go/pkg/modelruntime -count=1
go vet ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn

cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-model-runtime-bridge.service.spec.ts \
  --include=src/app/desktop/desktop-model-runtime-resource.service.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/tray-panel/tray-panel.spec.ts
```

These checks inspect, compile, and simulate lifecycle state. They do not start
LEM or load a model.

Repository entrypoints:

```bash
go tool wails3 task test:go
go tool wails3 task test:frontend
go tool wails3 task test

go tool wails3 task test:cover:go
go tool wails3 task test:cover:frontend
go tool wails3 task test:cover
```

The ordered frontend confidence gate used by CI is:

```bash
go tool wails3 task verify:frontend
```

It runs formatting, type/build checks, Angular tests, executable development
contracts, capability inventory, production build, and output verification in
one deterministic order.

Before stopping after code changes, run checks proportional to the scope:

```bash
gofmt -l go/
git diff --check
go vet ./go/...
go tool wails3 task test
cd frontend && npm run build
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

- `frontend/src/app/desktop/desktop-catalogue.data.ts`
- `frontend/src/app/desktop/surfaces/surface-registry.ts`
- `frontend/src/app/desktop/apps/app-view.ts`
- `frontend/src/app/desktop/desktop-route-tree.ts`

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
`frontend/src/kit/` and plugin descriptors whose `kind` is `lit`. Do not
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
