# Desktop Overlay Modularisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the Start/session menu, context menu, tray panel, notification stack, and command palette into focused Angular components while preserving every existing behaviour.

**Architecture:** `DesktopComponent` remains the stateful coordinator. Five `OnPush` presenters consume required signal inputs, emit typed intents, and expose only the panel/focus handles needed by the parent's existing positioning code.

**Tech Stack:** Angular 22 standalone components, signal inputs, function outputs, Vitest/jsdom, Sass, NgRx.

## Global Constraints

- Work inline on the owner-authorised `main` branch; use no subagents.
- Preserve every existing control, label, icon, tooltip, i18n marker, event, position, current action, and placeholder action.
- Do not move session, route, window, NgRx, persistence, Wails, CoreGO, fixture, or live-data behaviour.
- Use British English and retain EUPL-1.2 headers where present.
- Write presenter tests first and observe the missing-component failure.
- Keep `.playwright-mcp/` untouched.

---

### Task 1: Define the shared overlay contracts

**Files:**

- Modify: `frontend-ng/src/app/desktop/shell/shell.types.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`

**Interfaces:**

- Produces:
  - `ShellContextItem`
  - `ShellContextMenuState`
  - `ShellContextSubmenuState`
  - `ShellStartSubmenuState`
  - `ShellPosition`
  - `ShellTrayState`
  - `ShellTrayKey`
  - `ShellSessionAction`
  - `ShellNotification`
  - `ShellCommand`
  - `ShellPaletteState`
  - `ShellLanguage`
  - `ShellWorldClock`
  - `ShellChildRequest`

- [x] Replace the anonymous context, panel, notification, command, and submenu
  shapes in `DesktopComponent` with the shared interfaces after the presenter
  tests have established their public API.

### Task 2: Extract the Start/session menu

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/start-menu.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/start-menu.ts`
- Create: `frontend-ng/src/app/desktop/shell/start-menu.html`
- Create: `frontend-ng/src/app/desktop/shell/start-menu.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: routed `DesktopMenuCategory` rows, `ShellUserIdentity`,
  `ShellPosition`, primitive `ShellStartSubmenuState` fields, open-category
  state, and routed child tuples.
- Produces:
  - `categoryRequested: string`
  - `appRequested: DesktopMenuApp`
  - `appHovered: ShellValueEvent<DesktopMenuApp, MouseEvent>`
  - `childRequested: ShellChildRequest`
  - `sessionRequested: ShellValueEvent<ShellSessionAction>`
  - `panelElement: HTMLElement | undefined`

- [x] Write a test which renders one category, one child application, the
  identity panel, and submenu, then asserts the real category, app, child, and
  session-control outputs.
- [x] Run
  `npx ng test --watch=false --include=src/app/desktop/shell/start-menu.spec.ts`
  and observe failure because `ShellStartMenu` does not exist.
- [x] Implement the component with the unchanged markup and typed intents.
- [x] Replace the inline block and point `placeSession`/`placeSub` at the child
  panel handle.
- [x] Move only `.startpanel`, `.sm-*`, and `.sp-*` rules.
- [x] Run the Start-menu and desktop specs and observe them pass.

### Task 3: Extract the context menu

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/context-menu.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/context-menu.ts`
- Create: `frontend-ng/src/app/desktop/shell/context-menu.html`
- Create: `frontend-ng/src/app/desktop/shell/context-menu.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`

**Interfaces:**

- Consumes: primitive `ShellContextMenuState` and
  `ShellContextSubmenuState` fields plus the context-item array.
- Produces:
  - `submenuRequested: ShellValueEvent<number, MouseEvent>`
  - `submenuDismissed: void`
  - `itemRequested: ShellValueEvent<ShellContextItem>`
  - `panelElement: HTMLElement | undefined`

- [x] Write a test which renders a heading, action, separator, parent item, and
  nested action, then asserts hover, close, and selection outputs.
- [x] Run the focused test and observe the missing-component failure.
- [x] Implement the unchanged context-menu markup and outputs.
- [x] Replace the inline block and point `placeMb`, `placeCtxSub`, and
  `clampCtx` at the child panel handle.
- [x] Run the context-menu and desktop specs and observe them pass.

### Task 4: Extract the tray panel

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/tray-panel.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/tray-panel.ts`
- Create: `frontend-ng/src/app/desktop/shell/tray-panel.html`
- Create: `frontend-ng/src/app/desktop/shell/tray-panel.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: primitive `ShellTrayState` fields, language rows, selected
  language, clock, date, world clocks, formatted world-clock values, and power
  sparkline JSON.
- Produces:
  - `languageRequested: string`
  - `appRequested: { appId: string; subId?: string }`
  - `panelElement: HTMLElement | undefined`

- [x] Write a test which exercises the Language and Power variants and asserts
  the selected language and Control/Power launch intent.
- [x] Run the focused test and observe the missing-component failure.
- [x] Implement all four existing tray variants without changing copy or
  controls.
- [x] Replace the inline block and point `placeTray` at the child panel handle.
- [x] Move `.traypanel`, `.tr-*`, and `.trh` rules.
- [x] Run the tray-panel and desktop specs and observe them pass.

### Task 5: Extract notifications

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/notification-stack.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/notification-stack.ts`
- Create: `frontend-ng/src/app/desktop/shell/notification-stack.html`
- Create: `frontend-ng/src/app/desktop/shell/notification-stack.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: `readonly ShellNotification[]`.
- Produces: `dismissRequested: number`.

- [x] Write a test which renders title/body/icon content, omits an empty body,
  and emits the selected notification id from the real Dismiss button.
- [x] Run the focused test and observe the missing-component failure.
- [x] Implement the notification stack and replace the inline block.
- [x] Move the notification selectors and animation without changing
  taskbar-edge, shell-mode, reduced-motion, or light-mode behaviour.
- [x] Run the notification and desktop specs and observe them pass.

### Task 6: Extract the command palette

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/command-palette.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/command-palette.ts`
- Create: `frontend-ng/src/app/desktop/shell/command-palette.html`
- Create: `frontend-ng/src/app/desktop/shell/command-palette.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: primitive `ShellPaletteState` fields, `readonly ShellCommand[]`,
  clock/date text, and throughput/power sparkline JSON.
- Produces:
  - `queryChanged: Event`
  - `keyRequested: KeyboardEvent`
  - `commandRequested: number`
  - `selectionRequested: number`
  - `backdropRequested: Event`
  - `focusInput(): void`

- [x] Write tests which exercise input, ArrowDown, row hover, row selection,
  backdrop selection, selected-row styling, empty-result copy, and the
  empty-query time/throughput/power widgets.
- [x] Run the focused test and observe the missing-component failure.
- [x] Implement the palette and replace the inline block.
- [x] Point `openPalette` at `focusInput()` and retain all filtering, keyboard,
  command, and focus-restoration logic in the parent.
- [x] Move `.palette*` and `.pl-*` rules.
- [x] Run the palette and desktop specs and observe them pass.

### Task 7: Verify and commit the tranche

**Files:**

- Review every file changed by Tasks 1–6.

- [x] Run all five presenter specs and
  `src/app/desktop/desktop.component.spec.ts` together.
- [x] Run `cd frontend-ng && npm run build`.
- [x] Run Prettier on the new shell TypeScript, HTML, and spec files.
- [x] Run `git diff --check`.
- [x] Confirm `.playwright-mcp/` remains untouched.
- [x] Update `AGENTS.md` with the new overlay component map.
- [x] Commit with `refactor(frontend): modularise desktop overlays`.
