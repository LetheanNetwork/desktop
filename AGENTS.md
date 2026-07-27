<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Agent Notes

This repository builds the Lethean Desktop product binary, `lthn`. The binary
is a CLI router first. Wails desktop/mobile hosts and the Angular application
are consumers of the same Go service composition; they are not the binary's
identity. A frontend failure must not silently break `lthn serve`, `lthn ai`,
or the other non-GUI commands.

## Product and UI decisions

- `frontend-ng/` is the canonical and only product frontend.
- The frontend is Angular 22, standalone, client-side rendered, and
  hash-routed. Do not add Angular SSR, prerendering as an application mode,
  hydration, or a second frontend framework.
- Production Angular output goes directly to `go/cmd/lthn/dist/` and is
  embedded by `go/cmd/lthn/embed.go`.
- Wails owns native windows, tray lifetime, application policy, mobile hosts,
  and the Go/TypeScript transport. Angular owns the rendered UI and navigation.
- Use British English in code, copy, docs, and tests: `colour`, `behaviour`,
  `centre`, `organisation`, `licence`.
- The project is EUPL-1.2 and must not acquire feature paywalls or “Pro” gates.

### What the similarly named directories mean

- `frontend-ng/` — live Angular product source. Build, test, and develop here.
- `frontend/` — two tracked Wails mobile-generated support binding files. It is
  not a second application. iOS/Android tasks still mention this path, so do not
  delete it as cosmetic cleanup without repairing and testing those tasks.
- `frontend-ng/bindings/` — ignored generated Wails TypeScript bindings.
- `frontend-lit-ref/` — ignored local snapshot of the retired Lit application.
  It is byte-for-byte recoverable as `67b012f^:frontend` (361 files), not a
  build input or source of truth. The owner has chosen Git history, rather than
  a working-tree archive, as the retention policy for retired implementations.
- `docs/design/lit/` — tracked pre-Angular visual prototypes only.
  `docs/design/Lethean-5.zip` contains byte-identical copies of those files,
  and `docs/design/HANDOVER.md` duplicates
  `docs/design/lit/HANDOVER.md`. None are built, and new duplicate code
  archives must not be added; Git is the archive.

Lit is still an intentional runtime dependency inside the Angular project:
`frontend-ng/src/kit/` implements reusable custom elements with Lit, and the
plugin-view runtime supports descriptors whose `kind` is `lit`. Retiring the
old Lit application does **not** mean removing the `lit` package or those
framework-neutral elements.

### Design-system contract

The Lethean Design Pack is the visual and brand reference, not another product
frontend or a path-level dependency. It defines one dark-calm token engine,
Lethean's teal skin, the hoplite mark, Geist/Geist Mono/Instrument Serif roles,
Font Awesome iconography, quiet motion, and Vi's calm British-English voice.
Its `angular/desktop/` mock is the intended desktop/window UX and app catalogue;
its runnable Angular 18 workspace is a reference fixture, not code to restore.

The production ports already live in:

- `frontend-ng/src/foundations/` — the pack's CSS tokens converted to Sass,
  with self-hosted fonts for offline/CSP-safe builds and an Android profile.
- `frontend-ng/src/kit/` — the active typed Lit custom-element layer.
- `frontend-ng/src/app/desktop/` — the Angular 22 evolution of the pack's
  desktop mock, with NgRx, lazy routes, localisation, WebMCP, and Wails/CoreGO
  integration.

When applying a newer design pack, diff and port the relevant tokens, assets,
or interaction intent into those production locations. Do not copy the pack's
sample workspace into the repository, import its mock state as live product
data, or make the preview and product co-equal implementations. Preserve
shipped production adaptations unless the replacement is explicitly tested.

## Current repository shape

```text
desktop/
├── go.work                       # workspace contains ./go only
├── go/
│   ├── go.mod                    # dappco.re/lthn/desktop
│   ├── cmd/lthn/                 # CLI, composition root, embedded assets
│   └── pkg/                      # product services
├── frontend-ng/                  # canonical Angular application
├── frontend/                     # Wails mobile support bindings, not an app
├── build/                        # Wails/platform/task configuration
├── bundles/                      # marketplace bundle manifests
├── docs/
├── Taskfile.yml
├── CLAUDE.md
└── AGENTS.md
```

There is no `.gitmodules` file and no tracked `external/` checkout on `main`.
`go.work` uses only `./go`; CoreGO and its sibling capabilities resolve from
the versioned `dappco.re/go*` modules in `go/go.mod`. Do not restore the old
submodule topology or write new instructions that assume `external/*` exists.
Use `go.mod`, `go.sum`, and `go.work` as the dependency truth.

Version manifests are authoritative:

- `go/go.mod` — Go toolchain, Wails, CoreGO, and backend modules.
- `frontend-ng/package.json` and `package-lock.json` — Angular, TypeScript,
  NgRx, Lit, Wails runtime, xterm, and npm.
- `.github/workflows/build.yml` — CI toolchain and platform matrix.

At the time of the Angular migration the main stack is Go 1.26, Angular 22,
TypeScript 6, NgRx 21, npm, Wails 3 alpha, and the versioned
`dappco.re/go*` framework. Read the manifests before relying on a patch-level
version.

## Code map

### Go composition

- `go/cmd/lthn/main.go` — desktop CLI router. It dispatches `version`, `help`,
  `gui`, `tray`, `serve`, `ai`, `config`, `state`, `events`, `process`,
  `sessions`, `models`, `validate`, `firstlaunch`, `permissions`, `telemetry`,
  `service`, `api`, `fleet`, `opencode`, and `marketplace`.
- `go/cmd/lthn/app.go` — `newAppCore` and the application-wide service
  composition root.
- `go/cmd/lthn/embed.go` — embeds Angular `dist/` and native icons.
- `go/pkg/desktop/` — Wails application, windows, tray, deep links, SPA
  mounting, native policy, and runtime events.
- `go/pkg/connection/` — WebSocket transport used by the Wails runtime. Its
  generic loopback default is `ws://localhost:9099/wails/ws`; full
  `go tool wails3 task dev` moves the Lethean transport to 9199 because the
  development-only Wails MCP service owns 9099.
- `go/pkg/server/` and `go/pkg/api/` — HTTP gateway, route groups, OpenAPI, and
  SDK generation.
- `go/pkg/runner/` — inference-facing service used by CLI, server, and GUI.
- `go/pkg/appconfig/` — settings catalogue consumed by the Angular controls.
- `go/pkg/services/` — manual-by-default background service catalogue and
  lifecycle manager. It persists definitions through the registered
  application `io.Medium` and delegates every runtime operation to the named
  `go-process.Service`. Angular uses the `Lifecycle` Wails wrapper; native
  launchd/systemd installation remains an explicit separate compatibility
  path.
- `go/pkg/models/` — Medium-backed, path-private catalogue for `lem/models`;
  renderer references are opaque `model-…` IDs.
- `go/pkg/modelruntime/` — bounded LEM client and the sole renderer-facing
  model-runtime state machine. It coordinates the managed `inference`
  service, catalogue, credential, immutable snapshots, sampling, and explicit
  Start/Load/Unload/Restart/Stop operations.
- `go/pkg/telemetry/`, `go/pkg/fleet/`, `go/pkg/marketplace/`, and the other
  `go/pkg/*` directories — independently registered product services.

Add CLI verbs as flat `cmdX(args []string) int` handlers which delegate to
`go/pkg/*`; do not put reusable capability into `cmd/lthn`.

### Angular application

- `frontend-ng/src/main.ts` — direct Angular bootstrap.
- `frontend-ng/src/app/app.config.ts` — hash router, NgRx, transport, WebMCP,
  deep-link, and mobile initialisers.
- `frontend-ng/src/app/app.routes.ts` — top-level routes:
  `#/`, `#/w/:app`, and `#/tray`.
- `frontend-ng/src/app/desktop/desktop-catalogue.data.ts` — typed app,
  category, ordering, and child-navigation catalogue.
- `frontend-ng/src/app/desktop/dev-panel.data.ts` — typed CoreGO/IDE panel
  fixtures and route lookup.
- `frontend-ng/src/app/desktop/desktop-shell-fixtures.data.ts` — typed world
  clock and package-status fixtures.
- `frontend-ng/src/app/desktop/desktop.data.ts` — shared desktop state defaults
  and compatibility exports for the split data modules.
- `frontend-ng/src/app/desktop/apps/app-view.ts` — lazy component registry.
- `frontend-ng/src/app/desktop/desktop-route-tree.ts` — derives the router and
  menus from the app/category registries.
- `frontend-ng/src/app/desktop/window-manager.service.ts` — single source of
  truth for Angular window state.
- `frontend-ng/src/app/desktop/window-interaction.service.ts` — tested,
  stateless drag, resize, snap, marquee, group-drag, and grouping algorithms.
  `DesktopComponent` retains DOM pointer lifecycles and applies interaction
  results through the window manager.
- `frontend-ng/src/app/desktop/desktop-services-bridge.service.ts` — defensive
  managed-services Wails bridge. It accepts known service IDs and bounded
  policy/output requests, rejects execution-bearing responses, forwards
  `lthn:services:changed`, and makes no Wails call or event subscription in
  offline demo mode.
- `frontend-ng/src/app/desktop/desktop-model-runtime-resource.service.ts` —
  ref-counted shared ModelRuntime snapshot/event/poll resource used by Control
  and Telemetry, with deterministic in-memory lifecycle operations in offline
  demo mode.
- `frontend-ng/src/app/desktop/apps/control/control-services.view.ts` —
  Control's working Services interface under the stable internal `daemons`
  tab value. It presents manual Start/Stop/Restart and explicit bounded output
  without exposing command, arguments, environment, or working directories.
- `frontend-ng/src/app/desktop/shell/` — behaviour-preserving presenters for
  the menu bar, taskbar/dock, Start and context menus, tray panels,
  notifications, and command palette. `DesktopComponent` remains their
  coordinator; shell components receive typed inputs and emit interaction
  intents.
- `frontend-ng/src/app/desktop/surfaces/` — lazy product surfaces and shared
  bridge/page primitives.
- `frontend-ng/src/app/store/` — NgRx state which crosses components or
  transport boundaries.
- `frontend-ng/src/app/connection-manager.service.ts` — installs the Wails
  WebSocket transport before generated binding calls.
- `frontend-ng/src/app/desktop/desktop-mcp.service.ts` — Angular WebMCP tools.
- `frontend-ng/src/wails-bridge.ts` — unbootstrapped compatibility fallback;
  it is not the primary transport.
- `frontend-ng/src/foundations/` — design tokens and global foundations.
- `frontend-ng/src/kit/` — active Lit-based reusable custom elements.

When adding an Angular app surface, update `APPS`/`CATEGORIES` and
`APP_REGISTRY`, let `DESKTOP_APP_ROUTES` derive the route tree, and extend the
route/registry tests. Prefer standalone lazy components, `OnPush`, signals for
local reactive state, and NgRx for shared/event-driven state. Do not recreate
the retired custom-element view switcher.

## CoreGO development contract

The user's Go framework is the `dappco.re/go*` family. In product code, prefer
the local package's established CoreGO idiom over replacing it with generic
stdlib patterns:

- `core.Args`, `core.Exit`, `core.Print`, `core.Println`, and `core.ParseFlag`
  for process/CLI boundaries.
- `core.Result`, `core.Ok`, `core.Fail`, `core.E`, and `core.NewError` for the
  result and error contract where the package already uses it.
- `core.Core` service registration and lifecycle rather than package-level
  hidden wiring.
- Canonical services expose `Service`, `NewService(Options)`, a method
  `Register(*core.Core) core.Result`, and a free one-shot
  `Register(*core.Core) core.Result`, with a `// Usage example:` marker.

All file-backed product operations must ultimately flow through a registered
`dappco.re/go/io.Medium`. `io.Medium` is the security boundary: do not add a
raw `os`/`path/filepath`/`syscall`/Core-Fs fallback for local convenience,
including for metadata, tests, previews, recursion, or error recovery. An
unavailable Medium fails closed. Resolve provider roots and credentials in
trusted Go composition; renderer contracts carry only mount IDs and
provider-relative paths.

The canonical Files service is `go/pkg/office/files`. Its runtime metadata is
the versioned `desktop/files/runtime.json` document on the registered
application I/O Medium. Existing local directories may be composed as
`documents`, `downloads`, `projects`, `models`, `recordings`, and
`screenshots`; missing roots are skipped and must not be created by browsing.
Each mutable content Medium owns only its audited `.lthn-files` namespace.
Mutations emit `lthn:files:changed` with mount IDs and relative paths. Explicit
offline transport uses one isolated in-memory demo store per Files window and
must make no Wails call or event subscription.

Use TDD for new behaviour. New public symbols should carry focused
Good/Bad/Ugly tests and runnable examples in the matching package. Use
`*core.T`/the package's existing test convention, `t.TempDir()`, and
`t.Setenv("HOME", ...)` for user-data isolation. Never let a unit test write to
the real `~/Lethean/` tree.

Focused Files checks:

```bash
go test ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files.app.spec.ts \
  --include=src/app/desktop/desktop-files-bridge.service.spec.ts
```

Focused managed-services checks:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/services -count=1
go vet ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts \
  --include=src/app/desktop/apps/control/control-services.view.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts
```

Managed services never auto-start during registration, Core startup,
catalogue reads, UI refresh, or event subscription. Trusted definitions and
policy are stored at `desktop/services/catalogue.json` on the registered
application `io.Medium`; desired state, process identity, output, and errors
remain transient. Explicit Core/Desktop shutdown stops running services, while
closing windows leaves them alive under the tray-owned Core.

LEM is the fixed sibling `lem`/`lem.exe` managed-service executable. It starts
model-less on `127.0.0.1:36911` only after an explicit action:

```text
lem serve --addr 127.0.0.1:36911 --shutdown-timeout 10s
```

There is no PATH fallback, renderer endpoint, renderer credential, or native
renderer model path. Models live under `lem/models` on the registered
application Medium and the admin credential lives at `lem/admin.token`.
Unsupported runtime metrics remain absent; connected Angular surfaces render
`—` and empty series instead of substituting demo or benchmark values.

Focused model-runtime checks:

```bash
node --test scripts/verify-model-runtime-convergence.test.mjs
go test ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/services ./go/pkg/modelruntime -count=1
go vet ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-model-runtime-bridge.service.spec.ts \
  --include=src/app/desktop/desktop-model-runtime-resource.service.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/tray-panel/tray-panel.spec.ts
```

The external v0.9.0 audit currently reports a large pre-existing compliance
backlog; it is **not** a green all-zero gate on this branch. Run it as a
before/after no-regression diagnostic for changed Go scope. Do not claim the
repository is globally compliant, and do not expand a task into thousands of
unrelated mechanical rewrites.

## Build and development

Install the frontend exactly from its npm lockfile:

```bash
cd frontend-ng
npm ci
```

Frontend-only development:

```bash
cd frontend-ng
npm run demo
```

The expanded equivalent is:

```bash
cd frontend-ng
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
# http://127.0.0.1:9245/#/
```

For deterministic browser-only design work, place preview options before the
hash. `lthn-offline=1` disables the Wails socket/retry loop while keeping
fixture data visibly labelled; `lthn-view` accepts `desktop`, `shell`, or
`device`, and `lthn-device` accepts `small`, `large`, or `full`.

```text
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=shell#/
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=device&lthn-device=small#/
```

Full Wails development from the repository root:

```bash
go tool wails3 task dev
```

Development windows load the Angular loopback server directly and receive the
validated Wails WebSocket URL as `lthn-ws`; macOS WebKit rejects JavaScript
WebSockets started from the secure custom `wails://` scheme. The transport
allows only the exact loopback development origins derived from
`WAILS_VITE_PORT`. Production builds retain the embedded `wails://` asset
route, and must not inherit this development URL behaviour.

Native macOS development, test, server, and production Task entrypoints link
at the product's macOS 26.0 floor. `MACOS_DEPLOYMENT_TARGET` in the root
`Taskfile.yml` is authoritative; the Taskfiles must pass it through
`CGO_CFLAGS`, `CGO_CXXFLAGS`, and `CGO_LDFLAGS`, because
`MACOSX_DEPLOYMENT_TARGET` alone does not override Go 1.26's 11.0 final-link
default. The development and production plists must declare the matching
`26.0.0` minimum.

The macOS development and production plists carry user-facing Documents and
Downloads usage descriptions for the Files app. A denied protected-folder
mount remains unavailable through its `io.Medium`; never bypass that result
with raw host file access.

Run `task doctor` before development when the toolchain, generated bindings,
optional crew repositories, or ports are in doubt. Run `task verify:frontend`
for the same ordered Angular confidence gate used by CI.

Production Angular build:

```bash
cd frontend-ng
npm run build
# output: ../go/cmd/lthn/dist/index.html
```

Platform builds and packages are routed through `Taskfile.yml` and
`build/{darwin,linux,windows,ios,android}/`. The root pre-build can also stage
LEM, `lthn-mlx`, `lthn-agent`, and `lthn-ai`; their checkout locations are
overridable with `LTHN_LEM_REPO`, `LTHN_MLX_REPO`, `LTHN_AGENT_REPO`, and
`LTHN_AI_REPO`. `LTHN_LEM_BIN` supplies a prebuilt matching-platform LEM
binary. The macOS application bundle, Linux AppImage/nFPM packages, and
default Windows NSIS installer copy it beside `lthn`; missing optional source
leaves the runtime unavailable without blocking a GUI-only build.
Do not assume those optional sibling repositories are present in CI or on
another developer's machine.

## Tests and verification

Use focused tests while iterating:

```bash
go test ./go/pkg/<changed-package>
cd frontend-ng
npx ng test --watch=false --include=src/path/to/file.spec.ts
```

Repository entrypoints:

```bash
go tool wails3 task test:go
go tool wails3 task test:frontend
go tool wails3 task test

go tool wails3 task test:cover:go
go tool wails3 task test:cover:frontend
go tool wails3 task test:cover
```

Frontend CI tests are Angular's Vitest runner in jsdom. Specs are colocated as
`*.spec.ts`; reactive rendering generally needs an action followed by
`await fixture.whenStable()`. Preserve hash-router tests and keep Wails/WebMCP
tests independent of a live native runtime.

The frontend has an improvement target of at least 70% coverage, but the
current aggregate is below that target and no threshold is enforced by
`vitest-base.config.ts`. Always measure a fresh report; do not copy old
coverage percentages into plans or completion claims.

Before stopping after code changes, run the checks proportional to the scope:

```bash
gofmt -l go/
git diff --check
go vet ./go/...
go tool wails3 task test
cd frontend-ng && npm run build
```

The complete Go suite is large, noisy, and contains long security sweeps.
`pkg/account` alone can take roughly 80 seconds. A running development
`lthn.app` owns port `127.0.0.1:9099` and can make `pkg/desktop` fail with
“address already in use”; close the development app before that focused test or
record the environmental collision separately.

## Migration-retirement status

The active development topology is converged:

- binding generation targets only `frontend-ng/bindings`, synchronises the
  root Go workspace, carries platform-specific cache markers, and restores
  desktop bindings after mobile generation;
- CI performs a normal checkout with no recursive submodules;
- `build/audit.sh` uses `frontend-ng` and npm;
- active production Go comments point at Angular surfaces and
  `frontend-ng/bindings`; and
- `CLAUDE.md` defers to this contract while `docs/development.md` describes a
  normal clone, HMR, tests, builds, and the current transport ports.

The executable convergence contracts in
`scripts/verify-frontend-convergence.test.mjs` guard these decisions. A
disposable clean-checkout proof is recorded in
`docs/superpowers/plans/2026-07-26-migration-retirement.md`.

The tracked Lit design ZIP and duplicate handover remain reference archives,
and the ignored `frontend-lit-ref/` snapshot remains local user material.
Likewise, the two tracked files under `frontend/` remain mobile support
inputs. Do not delete reference material, mobile support files, or generated
bindings merely because their directory names look old.
