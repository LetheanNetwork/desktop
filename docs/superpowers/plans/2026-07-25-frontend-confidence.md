# Frontend Confidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Angular desktop honest about fixture/offline data, provide deterministic browser preview controls, and give developers one reliable environment and frontend verification path.

**Architecture:** Keep transport ownership in `ConnectionManagerService`, presentation-mode selection in `MobileRuntimeService`, and per-surface data provenance in `SurfacePage`. Add a dependency-injected Node doctor whose probes can be contract-tested without relying on the developer machine. Compose existing frontend checks into one `npm run verify` entrypoint and expose it through Task and CI.

**Tech Stack:** Angular 22 standalone components, signals, Vitest/jsdom, Node 22 test runner, Task, GitHub Actions, Wails 3.

## Global Constraints

- Work inline on `main`; the owner has explicitly authorised continuing after the merge and has asked for no subagents.
- Follow strict red-green-refactor for observable runtime behaviour.
- Use British English in code, messages, tests, and documentation.
- Preserve fixture content for design preview, but label it accurately whenever live data is unavailable.
- Do not change the Wails HMR flow or production embedding contract.
- Keep verification bounded: focused tests during each red-green cycle, one complete frontend verification at the end, then `git diff --check`.

---

### Task 1: Add an intentional offline transport mode

**Files:**

- Modify: `frontend/src/app/connection-manager.service.spec.ts`
- Modify: `frontend/src/app/connection-manager.service.ts`

- [x] Add a focused test proving `?lthn-offline=1` installs the Wails transport without opening a socket, reports an `offline` state, and rejects binding calls immediately with an offline-preview error.
- [x] Run only the connection-manager spec and observe the new test fail for missing behaviour.
- [x] Extend `ConnectionState` with `offline`, add `ConnectionManagerOptions.offline`, and expose a readonly `offline` computed signal.
- [x] Resolve offline mode from the explicit option or `lthn-offline` query value. Treat `1`, `true`, and `yes` as enabled.
- [x] In offline mode, skip initial connection and reconnection scheduling. Keep `call()` fail-fast and leave normal Wails/native behaviour unchanged.
- [x] Run only the connection-manager spec and observe it pass.

### Task 2: Add deterministic browser presentation controls

**Files:**

- Modify: `frontend/src/app/mobile-runtime.service.spec.ts`
- Modify: `frontend/src/app/mobile-runtime.service.ts`

- [x] Add focused tests proving web previews accept `lthn-view=shell`, `lthn-view=device&lthn-device=small`, and ignore invalid values.
- [x] Run only the mobile-runtime spec and observe the new tests fail.
- [x] Read the browser query through an injected location contract and apply valid `desktop`, `shell`, or `device` views plus valid `small`, `large`, or `full` device sizes.
- [x] Preserve native iOS/iPad/Android selection as the authoritative default when no preview override is supplied.
- [x] Run only the mobile-runtime spec and observe it pass.

### Task 3: Show fixture, live, loading, and offline surface provenance

**Files:**

- Modify: `frontend/src/app/desktop/surfaces/surface-page.spec.ts`
- Modify: `frontend/src/app/desktop/surfaces/surface-page.ts`
- Modify: `frontend/src/app/desktop/surfaces/surface-page.html`
- Modify: `frontend/src/app/desktop/surfaces/surface-page.scss`

- [x] Add component tests proving a bridge-backed fixture starts labelled `Fixture data`, changes to `Live data` after a successful load, and changes to `Offline · fixture kept` after the initial backend failure while retaining rows.
- [x] Run only the surface-page spec and observe the new assertions fail.
- [x] Add a typed `SurfaceDataState` signal and derive accessible status text from it.
- [x] Reset the state on config changes, mark loading before a request, live only after a successful response, and offline on every failed request including silent initial loads.
- [x] Render a compact status badge in the header whenever the surface has a bridge or endpoint. Keep the existing detailed notice for user-triggered failures.
- [x] Run only the surface-page spec and observe it pass.

### Task 4: Add a testable development doctor

**Files:**

- Create: `scripts/dev-doctor.mjs`
- Create: `scripts/dev-doctor.test.mjs`
- Modify: `Taskfile.yml`

- [x] Write Node contract tests around injected command, port, and path probes. Cover a healthy workspace, a missing required tool, an occupied port, and optional sibling repositories.
- [x] Run only `scripts/dev-doctor.test.mjs` and observe failure because the doctor does not exist.
- [x] Implement reusable probe/report functions plus the executable CLI. Check Node, npm, Go, Wails, Task, ports 9099/9199/9245, the npm lockfile, installed modules, generated bindings, and optional crew repositories.
- [x] Make required failures produce a non-zero exit code; make occupied development ports and absent optional sibling repositories visible warnings.
- [x] Add root `task doctor`.
- [x] Run only the doctor contract test and run `node scripts/dev-doctor.mjs` once against the current machine.

### Task 5: Compose one frontend verification contract

**Files:**

- Modify: `scripts/verify-frontend-convergence.test.mjs`
- Modify: `frontend/package.json`
- Modify: `build/Taskfile.yml`
- Modify: `Taskfile.yml`
- Modify: `.github/workflows/build.yml`

- [x] Add a contract test that executes the package verification script with an injected npm runner, proving the checks run once and in this order: capability audit, contracts, Angular tests, build, embedded-build verification.
- [x] Add contract coverage for the public `verify:frontend` Task entrypoint and CI invoking the same package contract.
- [x] Run the focused Node contract and observe the new assertions fail.
- [x] Add `npm run verify`, root `task verify:frontend`, and change the shared frontend dependency install to `npm ci`.
- [x] Add one frontend verification job to CI and make platform builds depend on it, avoiding a repeated Angular test suite in every platform matrix entry.
- [x] Run the focused convergence contract and observe it pass.

### Task 6: Complete the bounded verification

**Files:**

- Review all files changed by Tasks 1–5.

- [x] Run `cd frontend && npm run verify`.
- [x] Run `cd frontend && npx tsc --noEmit`.
- [x] Run `git diff --check`.
- [x] Inspect `git status --short`, confirm `.playwright-mcp/` remains untouched, and record exact results in the handoff.
