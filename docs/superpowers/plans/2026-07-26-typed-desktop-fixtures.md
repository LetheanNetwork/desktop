# Typed Desktop Fixtures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the application catalogue, developer-panel payloads, world clocks, and package rows into focused typed data files without changing desktop behaviour.

**Architecture:** Three static modules own catalogue metadata, strict developer-panel fixtures, and shell widget fixtures. `desktop.data.ts` remains the compatibility facade and `DesktopComponent` remains the runtime coordinator.

**Tech Stack:** Angular 22, TypeScript 6, Angular `$localize`, Vitest/jsdom, NgRx 21.

## Global Constraints

- Work inline on the owner-authorised `main` branch; use no subagents.
- Preserve every fixture value, order, localisation marker, route, and rendered
  behaviour.
- Keep `desktop.data.ts` compatibility exports for existing consumers.
- Keep timers, formatting, DOM work, routes, NgRx, Wails/CoreGO, and
  persistence out of the data modules.
- Do not change HTML or Sass in this tranche.
- Use British English and keep `.playwright-mcp/` untouched.
- Write focused tests first and observe the missing-module failure.

---

### Task 1: Define the typed fixture contracts with failing tests

**Files:**

- Test: `frontend-ng/src/app/desktop/desktop-catalogue.data.spec.ts`
- Test: `frontend-ng/src/app/desktop/dev-panel.data.spec.ts`
- Test: `frontend-ng/src/app/desktop/desktop-shell-fixtures.data.spec.ts`

**Interfaces:**

- Consumes:
  - `APPS`, `ORDER`, and `CATEGORIES`;
  - `DEV_PANEL_CATALOGUE` and `devPanelFor(route)`;
  - `CLOCKS` and `PKGS`.
- Produces behavioural contracts for reference integrity, developer-route
  coverage, valid table serialisation, IANA time zones, and package variants.

- [x] **Step 1: Write the three focused specs**

  The catalogue spec iterates literal ids and fails on any dangling order or
  category reference. It also collects `APPS` entries with `dev: true` and
  requires `devPanelFor(app.route).kind` not to be `empty`.

  The developer-panel spec parses every table panel's `cols` and `rows`,
  requires unique column keys, and requires every row key to have a column. It
  also verifies an unknown route returns the typed empty panel.

  The shell spec constructs `Intl.DateTimeFormat` for every clock time zone,
  requires unique clock cities and package names, and accepts only `''` or
  `'ok'` package variants.

- [x] **Step 2: Run the specs and verify RED**

  Run:

  ```bash
  cd frontend-ng
  npx ng test --watch=false \
    --include=src/app/desktop/desktop-catalogue.data.spec.ts \
    --include=src/app/desktop/dev-panel.data.spec.ts \
    --include=src/app/desktop/desktop-shell-fixtures.data.spec.ts
  ```

  Expected: compilation fails because the three focused data modules do not
  exist.

### Task 2: Extract the application catalogue

**Files:**

- Create: `frontend-ng/src/app/desktop/desktop-catalogue.data.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.data.ts`
- Modify: `frontend-ng/src/app/desktop/desktop-route-tree.ts`
- Modify: `frontend-ng/src/app/desktop/surfaces/surface-registry.ts`
- Modify: `scripts/frontend-capability-inventory.mjs`
- Modify direct production catalogue imports as identified by TypeScript.

**Interfaces:**

- Produces:
  - `AppDef`
  - `Category`
  - `AppNavItem`
  - `APPS`
  - `ORDER`
  - `CATEGORIES`
  - `CTRL_NAV`
  - `GAMES_NAV`
  - `SETTINGS_NAV`

- [x] **Step 1: Move the catalogue declarations unchanged**

  Preserve each literal and compose `SURFACE_APPS`, `SURFACE_CATEGORIES`, and
  `SURFACE_CATEGORY_APPS` in the same positions.

- [x] **Step 2: Retain the compatibility facade**

  Import and re-export catalogue values/types from `desktop.data.ts`, then use
  them unchanged in `DesktopData` and `DEFAULT_DESKTOP_DATA`.

- [x] **Step 3: Point canonical consumers at the new owner**

  Update the route tree, desktop coordinator, surface registry, reducer, route
  content, deep-link service, and desktop MCP service to import catalogue
  values or types from `desktop-catalogue.data.ts`. Point the source-based
  capability inventory at the same authoritative file.

### Task 3: Extract and type developer-panel fixtures

**Files:**

- Create: `frontend-ng/src/app/desktop/dev-panel.data.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/apps/dev-panel.app.ts`
- Modify: `frontend-ng/src/app/desktop/window-route-content.ts`

**Interfaces:**

- Produces:
  - `DevPanelRoute`
  - `DevPanelKind`
  - `DevPanelColumn`
  - strict table/tree/console/terminal/cards/kanban fixture variants
  - `DevPanelFixture`
  - `DevPanelView`
  - `DEV_PANEL_CATALOGUE`
  - `EMPTY_DEV_PANEL`
  - `devPanelFor(route: string): DevPanelView`

- [x] **Step 1: Move every panel literal into the typed catalogue**

  Replace raw table JSON strings with `JSON.stringify` over typed column and
  row literals while retaining the exact resulting payload.

- [x] **Step 2: Remove `DEVPANEL: any` from the component**

  Make `panelFor` return `devPanelFor(APPS[window.app]?.route ?? '')`.

- [x] **Step 3: Type the developer-panel input path**

  Change `DevPanelApp.panel` from `any` to `DevPanelView` and
  `WindowRouteContent.panel` from `unknown` to `DevPanelView`, using
  `EMPTY_DEV_PANEL` as the default.

### Task 4: Extract shell clock and package fixtures

**Files:**

- Create: `frontend-ng/src/app/desktop/desktop-shell-fixtures.data.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.data.ts`

**Interfaces:**

- Produces:
  - `WorldClockFixture`
  - `PackageStatusVariant`
  - `PackageFixture`
  - `CLOCKS`
  - `PKGS`

- [x] **Step 1: Move localised clock and package rows unchanged**

  Export readonly arrays using `satisfies` and retain the existing city, zone,
  time-zone, package, state, and variant values.

- [x] **Step 2: Replace component-owned arrays**

  Expose `readonly clocks = CLOCKS` and `readonly pkgs = PKGS`; retain
  `worldClocks` and `fmtTz` in the component.

- [x] **Step 3: Remove duplicate unlocalised declarations**

  Re-export the new constants through `desktop.data.ts` rather than retaining
  a second clock/package source.

### Task 5: Verify, document, and commit

**Files:**

- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-07-26-typed-desktop-fixtures.md`
- Review every file changed by Tasks 1–4.

**Interfaces:**

- Produces an updated code map naming each data owner.

- [x] **Step 1: Run the focused confidence gate**

  Run:

  ```bash
  cd frontend-ng
  npx ng test --watch=false \
    --include=src/app/desktop/desktop-catalogue.data.spec.ts \
    --include=src/app/desktop/dev-panel.data.spec.ts \
    --include=src/app/desktop/desktop-shell-fixtures.data.spec.ts \
    --include=src/app/desktop/desktop.component.spec.ts \
    --include=src/app/app.routes.spec.ts
  ```

  Require every focused spec to pass.

- [x] **Step 2: Update the project code map**

  Add the catalogue, developer-panel fixture, and shell-fixture files to
  `AGENTS.md`; describe `desktop.data.ts` as the state/token compatibility
  facade.

- [x] **Step 3: Format and inspect**

  Run Prettier on changed TypeScript/spec files, then:

  ```bash
  git diff --check
  git status --short
  git diff --stat
  ```

  Confirm no HTML/Sass change and `.playwright-mcp/` remains unrelated.

- [x] **Step 4: Run the complete frontend gate**

  Run:

  ```bash
  cd frontend-ng
  npm run test:ci
  npm run build
  ```

  Require zero test failures and a successful production build.

- [x] **Step 5: Commit**

  Stage only the planned files and commit with:

  ```bash
  git commit -m "refactor(frontend): extract typed desktop fixtures"
  ```
