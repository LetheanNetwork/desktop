# Desktop Shell Modularisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the desktop menu bar and taskbar/dock into focused Angular components while preserving every existing UX feature and behaviour.

**Architecture:** `DesktopComponent` remains the stateful coordinator. New `OnPush` presentation components receive required signal inputs and emit typed interaction requests that the parent delegates to its existing methods.

**Tech Stack:** Angular 22 standalone components, signal inputs, function outputs, Vitest/jsdom, Sass, NgRx.

## Global Constraints

- Work inline on the owner-authorised `main` branch; use no subagents.
- Preserve every control, label, icon, tooltip, route, current action, and placeholder action.
- Do not move NgRx, window, session, persistence, Wails, or CoreGO behaviour.
- Use British English and retain EUPL-1.2 headers where present.
- Use focused red-green tests while editing, then one Angular build and one diff check.

---

### Task 1: Define typed shell presentation contracts

**Files:**

- Create: `frontend-ng/src/app/desktop/shell/shell.types.ts`

**Interfaces:**

- Produces:
  - `ShellUserIdentity`
  - `ShellWindowGroup`
  - `ShellValueEvent<T, E extends Event = Event>`

- [ ] Add the immutable user, group, and value/event interfaces used by both presenters and `DesktopComponent`.

```ts
export interface ShellUserIdentity {
  readonly initials: string;
  readonly name: string;
  readonly email: string;
  readonly host: string;
}

export interface ShellWindowGroup {
  id: string;
  name: string;
  ids: string[];
  apps: string[];
  open: boolean;
}

export interface ShellValueEvent<T, E extends Event = Event> {
  readonly value: T;
  readonly event: E;
}
```

### Task 2: Extract the menu bar

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/menu-bar.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/menu-bar.ts`
- Create: `frontend-ng/src/app/desktop/shell/menu-bar.html`
- Create: `frontend-ng/src/app/desktop/shell/menu-bar.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: `DesktopMenuCategory`, `ShellValueEvent`
- Produces:
  - `menuRequested: ShellValueEvent<string>`
  - `menuHovered: ShellValueEvent<string, MouseEvent>`
  - `categoryRequested: ShellValueEvent<DesktopMenuCategory>`
  - `trayRequested: ShellValueEvent<'lang' | 'wifi' | 'battery' | 'clock'>`

- [ ] Write a failing component test that sets active app/category/language/clock inputs, verifies the existing labels, clicks real buttons, and asserts the emitted values and original DOM events.
- [ ] Run `npx ng test --watch=false --include=src/app/desktop/shell/menu-bar.spec.ts` and observe the missing component failure.
- [ ] Implement `ShellMenuBar` with required signal inputs, function outputs, `OnPush`, `display: contents`, and the unchanged menu-bar markup.
- [ ] Replace the inline menu-bar block with `<lthn-shell-menu-bar>` and delegate each output to the existing parent method.
- [ ] Move only menu-bar-owned selectors into `menu-bar.scss`.
- [ ] Run the focused menu-bar and existing desktop specs and observe them pass.

### Task 3: Extract the taskbar and dock

**Files:**

- Test: `frontend-ng/src/app/desktop/shell/taskbar-dock.spec.ts`
- Create: `frontend-ng/src/app/desktop/shell/taskbar-dock.ts`
- Create: `frontend-ng/src/app/desktop/shell/taskbar-dock.html`
- Create: `frontend-ng/src/app/desktop/shell/taskbar-dock.scss`
- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.component.html`
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes: `AppDef`, `Win`, `ShellUserIdentity`, `ShellWindowGroup`, `ShellValueEvent`
- Produces:
  - `sessionRequested: Event`
  - `taskRequested: string`
  - `dockRequested: string`
  - `dockContextRequested: ShellValueEvent<string, MouseEvent>`
  - `groupRequested: string`
  - `groupContextRequested: ShellValueEvent<ShellWindowGroup, MouseEvent>`

- [ ] Write a failing component test that renders a user, two tasks, one running app, and one group, then verifies real click/context-menu outputs and the unchanged Trash control.
- [ ] Run `npx ng test --watch=false --include=src/app/desktop/shell/taskbar-dock.spec.ts` and observe the missing component failure.
- [ ] Implement `ShellTaskbarDock` with required signal inputs, function outputs, `OnPush`, `display: contents`, and unchanged taskbar/dock markup.
- [ ] Replace the two inline blocks with `<lthn-shell-taskbar-dock>` and delegate outputs to the existing parent methods.
- [ ] Replace anonymous user/group state shapes in `DesktopComponent` with the shared interfaces.
- [ ] Move taskbar/dock-owned selectors into `taskbar-dock.scss`, retaining shared and outer-layout selectors in the parent stylesheet.
- [ ] Run both presenter specs and the existing desktop spec and observe them pass.

### Task 4: Verify and commit the tranche

**Files:**

- Review all files changed by Tasks 1–3.

- [ ] Run the three focused Angular specs together.
- [ ] Run `cd frontend-ng && npm run build`.
- [ ] Run `git diff --check`.
- [ ] Confirm `.playwright-mcp/` remains untouched.
- [ ] Commit with `refactor(frontend): modularise desktop shell chrome`.
