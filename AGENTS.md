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
- `go/pkg/connection/` — WebSocket transport used by the Wails runtime. The
  default development endpoint is `ws://localhost:9099/wails/ws`.
- `go/pkg/server/` and `go/pkg/api/` — HTTP gateway, route groups, OpenAPI, and
  SDK generation.
- `go/pkg/runner/` — inference-facing service used by CLI, server, and GUI.
- `go/pkg/appconfig/` — settings catalogue consumed by the Angular controls.
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
- `frontend-ng/src/app/desktop/desktop.data.ts` — app and category metadata.
- `frontend-ng/src/app/desktop/apps/app-view.ts` — lazy component registry.
- `frontend-ng/src/app/desktop/desktop-route-tree.ts` — derives the router and
  menus from the app/category registries.
- `frontend-ng/src/app/desktop/window-manager.service.ts` — single source of
  truth for Angular window state.
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

Use TDD for new behaviour. New public symbols should carry focused
Good/Bad/Ugly tests and runnable examples in the matching package. Use
`*core.T`/the package's existing test convention, `t.TempDir()`, and
`t.Setenv("HOME", ...)` for user-data isolation. Never let a unit test write to
the real `~/Lethean/` tree.

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
wails3 task dev
```

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
`lthn-mlx`, `lthn-agent`, and `lthn-ai`; their checkout locations are
overridable with `LTHN_MLX_REPO`, `LTHN_AGENT_REPO`, and `LTHN_AI_REPO`.
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
wails3 task test:go
wails3 task test:frontend
wails3 task test

wails3 task test:cover:go
wails3 task test:cover:frontend
wails3 task test:cover
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
wails3 task test
cd frontend-ng && npm run build
```

The complete Go suite is large, noisy, and contains long security sweeps.
`pkg/account` alone can take roughly 80 seconds. A running development
`lthn.app` owns port `127.0.0.1:9099` and can make `pkg/desktop` fail with
“address already in use”; close the development app before that focused test or
record the environmental collision separately.

## Known main-branch migration drift

Treat these as known debt, not as canonical instructions:

- `build/Taskfile.yml` still passes the removed
  `../external/gui/go/...` path to `generate:bindings`; a clean binding
  regeneration/CI checkout needs that command repaired before `-clean=true`
  is trusted.
- `.github/workflows/build.yml` still describes and checks out recursive
  submodules even though this tree has no `.gitmodules`.
- `build/audit.sh` still runs `bun` commands under the removed `frontend/`
  application. Do not use it as the current all-in-one gate until it targets
  `frontend-ng`/npm.
- Several Go comments still point to `frontend/src/lit` or
  `frontend/bindings`; map them to the Angular surface or
  `frontend-ng/bindings` before treating the comment as a contract.
- `CLAUDE.md`, `docs/development.md`, and parts of other prose still describe
  the removed `external/` workspace or pre-Wails scaffold state.
- The tracked Lit design ZIP and duplicate handover are redundant archives;
  the ignored `frontend-lit-ref/` snapshot is local user material.

Fix these coherently as a migration-retirement change with tests. Do not
silently delete reference material, mobile support files, or generated
bindings just because their directory names look old.
