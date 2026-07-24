<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Wails WebSocket Connection Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route all generated Wails bindings and events through a secure,
configurable connection-manager service with the default endpoint
`ws://localhost:9099/wails/ws`.

**Architecture:** `pkg/connection` implements the Wails transport interfaces
and owns the HTTP/WebSocket lifecycle. `pkg/desktop` consumes the transport
without owning it, while an Angular root service implements
`RuntimeTransport` and selects a loopback or secure proxied URL at runtime.

**Tech Stack:** Go 1.26, CoreGO `core.Result`/`core.E`, Wails v3
`application.Transport`, Gorilla WebSocket, Angular 22 signals and dependency
injection, `@wailsio/runtime`, Vitest.

## Global Constraints

- Use UK English and `core.E` for Go errors.
- Do not use git.
- Preserve workspace-mode resolution through root `go.work`.
- Keep the default listener on `127.0.0.1:9099`.
- Require WSS for non-loopback frontend URLs.
- Never publish `LTHN_WAILS_WS_TOKEN` in served JavaScript or status.
- Add tests before each behaviour change and observe the focused failure.
- Finish with the full build and test gates from `AGENTS.md`.

---

### Task 1: Complete the backend service contract

**Files:**

- Modify: `go/pkg/connection/service_test.go`
- Modify: `go/pkg/connection/transport_internal_test.go`
- Modify: `go/pkg/connection/service.go`
- Modify: `go/pkg/connection/transport.go`
- Modify: `go/pkg/connection/service_example_test.go`

**Interfaces:**

- Produces: `NewService(Options) *Service`
- Produces: `(*Service).Register(*core.Core) core.Result`
- Produces: `Register(*core.Core) core.Result` whose value is `*Service`
- Produces: `(*Service).Transport() application.Transport`
- Produces: Wails `Transport`, `AssetServerTransport`, and
  `WailsEventListener` implementations.

- [x] Add AX-shaped tests proving the factory returns the service, environment
  defaults resolve correctly, relative/insecure/credential-bearing URLs fail,
  and JavaScript configuration contains the public URL but not the token.

- [x] Run `go test ./pkg/connection` and verify the new assertions fail for
  the missing canonical return value, URL validation, or token redaction.

- [x] Make registration return `core.Ok(s)`, harden URL/path/listen
  validation, redact secrets from `JSClient`, accept bearer authentication,
  and preserve `core.E` at Wails interface boundaries.

- [x] Add transport tests for exact Wails JSON field names, protocol errors,
  client limits, bounded queues, and clean/idempotent shutdown.

- [x] Run `go test -race ./pkg/connection` and require exit status 0.

### Task 2: Complete the Angular runtime transport

**Files:**

- Modify: `frontend-ng/src/app/connection-manager.service.spec.ts`
- Modify: `frontend-ng/src/app/connection-manager.service.ts`
- Modify: `frontend-ng/src/app/app.config.ts`
- Modify: `frontend-ng/src/index.html`

**Interfaces:**

- Produces: `ConnectionManagerService implements RuntimeTransport`.
- Produces: injectable `CONNECTION_MANAGER_OPTIONS`,
  `CONNECTION_LOCATION`, and `CONNECTION_SOCKET_FACTORY`.
- Consumes: backend `globalThis.__LTHN_CONNECTION__.webSocketUrl`.

- [x] Add Vitest cases asserting requests use `webviewWindowName`, configured
  URLs persist without tokens, remote `ws://` and credential-bearing URLs are
  rejected, request limits/timeouts reject deterministically, malformed input
  updates `lastError`, and disconnect/destroy suppress reconnect.

- [x] Run
  `npx ng test --configuration=ci --include=src/app/connection-manager.service.spec.ts`
  and observe the targeted failures.

- [x] Correct the Wails request envelope, validate URLs and options, bound
  pending calls, persist URL-only configuration, and keep bootstrap
  non-blocking while reconnecting in the background.

- [x] Re-run the focused Angular test command and require all cases to pass.

- [x] Run `npx ng build` and require exit status 0.

### Task 3: Canonically wire connection into the desktop binary

**Files:**

- Modify: `go/cmd/lthn/app.go`
- Modify: `go/cmd/lthn/main.go`
- Modify: `go/pkg/desktop/desktop_test.go`
- Modify: `go/pkg/desktop/gui_runtime.go`
- Modify: `go/pkg/desktop/desktop.go`

**Interfaces:**

- Consumes: Core service name `connection`.
- Consumes: `desktop.Options.Connection *connection.Service`.
- Produces: `application.Options.Transport` set to the connection manager's
  transport.

- [x] Add tests that retrieve `*connection.Service` through Core and that
  `guiApplicationOptions` preserves the supplied transport.

- [x] Run `go test ./pkg/desktop ./cmd/lthn` and observe the targeted failure.

- [x] Register `connection.Register` in `newAppCore`, resolve that instance in
  `cmdGUI`, and retain the explicit required dependency in `desktop.Run`.

- [x] Re-run `go test ./pkg/desktop ./cmd/lthn` and require exit status 0.

### Task 4: Document deployment configuration

**Files:**

- Modify: `docs/architecture.md`
- Modify: `docs/development.md`

**Interfaces:**

- Documents: default endpoint, six environment variables, WSS proxy topology,
  token/origin rules, mobile/browser URL configuration, and secret handling.

- [x] Add a connection-manager architecture section and a concise reverse
  proxy/configuration runbook using the exact environment names from the
  design.

- [x] Search the documentation for `LTHN_WAILS_WS_` and verify all supported
  settings and the no-secret-in-JavaScript rule are present.

### Task 5: Full verification

**Files:**

- Verify all modified Go, TypeScript, HTML, and Markdown files.

- [x] Run `gofmt -w` only on changed Go files, then run `gofmt -l` on those
  exact paths and require no output.

- [x] Run `go work sync`.

- [x] Run `go vet ./go/...`.

- [x] Run `wails3 task test` and require both Go and frontend suites green.

- [x] Run `npx ng build` from `frontend-ng/`.

- [x] Run the v0.9.0 audit without invoking git. Record the repository's
  existing compliance backlog separately; verify the new package has zero
  AX-triplet and example gaps and introduces no banned imports or Result
  discards.

- [x] Run a final focused `go test -race ./pkg/connection` and the focused
  Angular connection-manager test so completion evidence is fresh.
