<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Shared Desktop Settings Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Control's Configuration surface use the same typed, persisted NgRx/RxJS desktop-control state as Settings while removing its obsolete fixture path.

**Architecture:** Add one standalone smart panel that selects from `desktopControlsFeature` and dispatches `desktopControlsActions`. Keep `DesktopControlsEffects` and `DesktopControlsBridgeService.SetMany` as the only async persistence path, then embed the panel in both Settings and Control.

**Tech Stack:** Angular 22 standalone components, NgRx 21 Store/Effects, RxJS, Vitest, TypeScript 6, Go `appconfig.Service.SetMany`, `io.Medium` persistence.

## Global Constraints

- Retain NgRx and RxJS as the renderer data store and backend synchronisation point.
- Angular signals may adapt selectors for templates but must not replace the store/effects pipeline.
- Offline demo mode must use its isolated in-memory snapshot and make no Wails call.
- Persist the complete bounded draft atomically with `SetMany`; never write individual inputs directly.
- Preserve both applications' existing UX, native-permission boundary, British English, and EUPL-1.2 licensing.
- Do not push or open a pull request as part of this plan.

---

### Task 1: Shared NgRx controls panel

**Files:**

- Create: `frontend/src/app/desktop/desktop-controls-panel.view.ts`
- Create: `frontend/src/app/desktop/desktop-controls-panel.view.spec.ts`

**Interfaces:**

- Consumes: `desktopControlsFeature`, `selectDesktopControlGroups`, `selectDirtyDesktopControlChanges`, `selectHasDirtyDesktopControls`, and `desktopControlsActions`.
- Produces: `DesktopControlsPanelView`, selector `lthn-desktop-controls-panel`, inputs for headings, excluded keys, action visibility, permission snapshots, and permission request state, plus a typed permission-request output.
- Produces truthful store/transport status plus optional configuration
  precedence and help copy without coupling the editor to Control's telemetry
  resource.

- [ ] **Step 1: Write failing component tests**

  Cover grouped rendering, toggle/select/number/text edit actions, apply/discard/reset actions, retry after failure, saving-state disabling, restart feedback, key exclusion, and explicit permission requests.

- [ ] **Step 2: Verify the tests fail for the missing component**

  Run:

  ```bash
  cd frontend
  npx ng test --watch=false --include=src/app/desktop/desktop-controls-panel.view.spec.ts
  ```

  Expected: failure because `DesktopControlsPanelView` does not exist.

- [ ] **Step 3: Implement the minimal shared panel**

  Use `Store.selectSignal` only as a rendering adapter. Dispatch
  `editControl`, `applyDraft`, `discardDraft`, `resetDraft`, and `load`; keep
  all bridge calls inside the existing RxJS effects.

- [ ] **Step 4: Verify the shared-panel tests pass**

  Run the focused command from Step 2 and require zero failed tests.

### Task 2: Adopt the shared panel in Settings and Control

**Files:**

- Modify: `frontend/src/app/desktop/apps/settings.app.ts`
- Modify: `frontend/src/app/desktop/apps/settings.app.spec.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-settings.view.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-secondary-views.spec.ts`
- Modify: `frontend/src/app/desktop/apps/control.app.ts`
- Modify: `frontend/src/app/desktop/apps/control.app.spec.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-view.models.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-view-state.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-view-state.spec.ts`
- Modify: `frontend/src/app/desktop/apps/control/control-demo.data.ts`
- Modify: `frontend/src/app/desktop/desktop-live-data.service.ts`
- Modify: `frontend/src/app/desktop/desktop-live-data.service.spec.ts`

**Interfaces:**

- Settings passes its curated-key exclusion and native permission state to `DesktopControlsPanelView` while retaining its existing top-level draft actions.
- Control's existing `ControlSettingsView` becomes a layout-preserving wrapper around the shared panel with its own explicit draft actions.
- `ControlLiveSnapshot` stops carrying settings because the NgRx/RxJS store owns that data independently.

- [ ] **Step 1: Replace fixture expectations with failing store-backed tests**

  Prove Control renders values supplied by NgRx selectors, edits a typed value,
  and dispatches `applyDraft` with the full dirty-change array. Prove the live
  Control aggregate reads telemetry, benchmarks, and processes only.

- [ ] **Step 2: Verify the changed tests fail against fixture-backed Control**

  Run:

  ```bash
  cd frontend
  npx ng test --watch=false \
    --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
    --include=src/app/desktop/apps/control.app.spec.ts \
    --include=src/app/desktop/apps/settings.app.spec.ts \
    --include=src/app/desktop/desktop-live-data.service.spec.ts
  ```

  Expected: failures because Control does not yet consume the store panel and
  the live aggregate still reads appconfig directly.

- [ ] **Step 3: Integrate the shared panel and delete only obsolete fixture wiring**

  Replace duplicated generic Settings controls with the panel, wrap it in
  Control's Configuration view, and remove `ControlSettingsViewModel`, demo
  settings, `mergeSettings`, `commit-settings`, and the duplicate controls
  dependency from `DesktopLiveDataService`.

- [ ] **Step 4: Verify the focused integration tests pass**

  Run the focused command from Step 2 and require zero failed tests.

### Task 3: Documentation and proportional verification

**Files:**

- Modify: `TODO.md`
- Modify: `AGENTS.md`

**Interfaces:**

- Document the NgRx/RxJS ownership boundary and mark Control configuration persistence complete through atomic `SetMany`.

- [ ] **Step 1: Update the project contracts**

  Record the shared panel and effects/store boundary in `AGENTS.md`; mark the
  Control configuration item complete in `TODO.md` and name `SetMany` rather
  than the single-setting compatibility method.

- [ ] **Step 2: Format and run focused checks**

  ```bash
  cd frontend
  npx prettier --write \
    src/app/desktop/desktop-controls-panel.view.ts \
    src/app/desktop/desktop-controls-panel.view.spec.ts \
    src/app/desktop/apps/settings.app.ts \
    src/app/desktop/apps/settings.app.spec.ts \
    src/app/desktop/apps/control.app.ts \
    src/app/desktop/apps/control.app.spec.ts \
    src/app/desktop/apps/control/control-settings.view.ts \
    src/app/desktop/apps/control/control-secondary-views.spec.ts \
    src/app/desktop/apps/control/control-view.models.ts \
    src/app/desktop/apps/control/control-view-state.ts \
    src/app/desktop/apps/control/control-view-state.spec.ts \
    src/app/desktop/apps/control/control-demo.data.ts \
    src/app/desktop/desktop-live-data.service.ts \
    src/app/desktop/desktop-live-data.service.spec.ts
  npx ng test --watch=false \
    --include=src/app/desktop/desktop-controls-panel.view.spec.ts \
    --include=src/app/desktop/apps/settings.app.spec.ts \
    --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
    --include=src/app/desktop/apps/control.app.spec.ts \
    --include=src/app/desktop/apps/control/control-view-state.spec.ts \
    --include=src/app/desktop/desktop-live-data.service.spec.ts \
    --include=src/app/store/desktop-controls.reducer.spec.ts \
    --include=src/app/store/desktop-controls.effects.spec.ts
  npm run build
  cd ..
  git diff --check
  ```

  Require successful exit codes and no failed tests or Angular build errors.

- [ ] **Step 3: Review the final diff without publishing it**

  ```bash
  git status --short --branch
  git diff --stat
  git diff -- frontend go TODO.md AGENTS.md docs/superpowers
  ```

  Confirm the branch contains only the shared settings work and leave it local.
