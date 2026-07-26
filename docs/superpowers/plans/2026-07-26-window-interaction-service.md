# Window Interaction Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move drag, resize, snap, marquee, group-drag, and grouping algorithms into a deterministic Angular service while preserving the desktop UX.

**Architecture:** A root-provided, stateless `WindowInteractionService` computes immutable windows, groups, and transient overlay values from typed input snapshots. `DesktopComponent` remains the DOM/event coordinator and applies results through the existing NgRx-backed `WindowManagerService`.

**Tech Stack:** Angular 22 standalone application, TypeScript 6, NgRx 21, Vitest/jsdom.

## Global Constraints

- Work inline on the owner-authorised `main` branch; use no subagents.
- Preserve every existing interaction, threshold, bound, snap transition,
  group action, focus change, and persistence point.
- Keep NgRx as the only durable window-state owner.
- Do not move DOM queries or global pointer-listener registration into the
  service.
- Do not change desktop markup, styling, routes, app surfaces, Wails/CoreGO,
  fixtures, or native-window behaviour.
- Use British English and keep `.playwright-mcp/` untouched.
- Write the service test first and observe its missing-module failure.

---

### Task 1: Define and test the interaction engine

**Files:**

- Test: `frontend-ng/src/app/desktop/window-interaction.service.spec.ts`
- Create: `frontend-ng/src/app/desktop/window-interaction.service.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.data.ts`

**Interfaces:**

- Produces:
  - `InteractionPoint` and `InteractionRect`
  - `MarqueeSession`, `MarqueeCandidate`, and `MarqueeUpdate`
  - `DragSession`, `DragUpdate`, and `SnapPreview`
  - `ResizeSession`
  - `GroupDragSession`, `GroupDragUpdate`, and `GroupProxy`
  - `KeyboardSnapDirection` and `KeyboardSnapIntent`
  - `WindowGroupingState`
  - `WindowInteractionService.beginMarquee(origin, host)`
  - `WindowInteractionService.moveMarquee(session, pointer, candidates)`
  - `WindowInteractionService.beginDrag(window, pointer, layer, host)`
  - `WindowInteractionService.moveDrag(session, pointer, window)`
  - `WindowInteractionService.beginResize(window, pointer)`
  - `WindowInteractionService.moveResize(session, pointer, window)`
  - `WindowInteractionService.beginGroupDrag(ids, windows, pointer, host)`
  - `WindowInteractionService.moveGroupDrag(session, pointer, windows, dock)`
  - `WindowInteractionService.snapRect(zone, bounds)`
  - `WindowInteractionService.snapWindow(window, zone, bounds)`
  - `WindowInteractionService.unsnapWindow(window)`
  - `WindowInteractionService.keyboardSnap(window, direction)`
  - `WindowInteractionService.createGroup(state, ids, name, id?)`
  - `WindowInteractionService.toggleGroup(state, id, nextZ)`
  - `WindowInteractionService.splitGroup(state, id, nextZ)`
  - `WindowInteractionService.closeGroup(state, id)`

- [x] **Step 1: Write the failing contract tests**

  Build literal fixtures for an 800-by-600 window layer, multiple window
  rectangles, two selected windows, and one group. Assert normalised marquee
  geometry, selected ids, drag constraints, snap preview, minimum resize,
  snap/unsnap restore state, keyboard intents, group movement, dock hit state,
  and all group transitions without asserting on mocks.

- [x] **Step 2: Run the new service spec and verify RED**

  Run:

  ```bash
  cd frontend-ng
  npx ng test --watch=false --include=src/app/desktop/window-interaction.service.spec.ts
  ```

  Expected: compilation fails because
  `./window-interaction.service` does not exist.

- [x] **Step 3: Add the typed runtime snap state**

  Add this existing runtime state to `Win`:

  ```ts
  export type WindowSnapState =
    | 'top'
    | 'max'
    | 'left'
    | 'right'
    | 'tl'
    | 'tr'
    | 'bl'
    | 'br';

  snapState?: WindowSnapState | null;
  ```

- [x] **Step 4: Implement the minimal stateless service**

  Use `@Injectable({ providedIn: 'root' })`. Keep the 4-pixel marquee
  threshold, 22-pixel snap threshold, 60/40 drag visibility bounds, 360/260
  resize minimums, exact keyboard table, and immutable group transitions as
  literal service policy. Do not inject `WindowManagerService`, the DOM, or
  persistence.

- [x] **Step 5: Run the service spec and verify GREEN**

  Run the Task 1 command and require every interaction test to pass without
  warnings or unhandled errors.

### Task 2: Replace component algorithms with service coordination

**Files:**

- Modify: `frontend-ng/src/app/desktop/desktop.component.ts`
- Test: `frontend-ng/src/app/desktop/desktop.component.spec.ts`

**Interfaces:**

- Consumes: every Task 1 service method and result type.
- Preserves: the existing template method names `startMarquee`,
  `startDrag`, `startResize`, `toggleGroup`, `splitGroup`, and `closeGroup`.

- [x] **Step 1: Inject the service and remove duplicate algorithms**

  Inject `WindowInteractionService` with `inject()`. Delete `hit` and the
  component's geometry/transition tables. Convert the existing methods into
  pointer-lifecycle coordinators which read DOM rectangles and apply returned
  windows/groups/focus through `WindowManagerService`.

- [x] **Step 2: Keep state application explicit**

  Add one private grouping-state application helper which assigns returned
  windows, groups, selected ids, and focus, then calls the existing
  `persist()`. Keep localised group-name construction in the component.

- [x] **Step 3: Run focused integration tests**

  Run:

  ```bash
  cd frontend-ng
  npx ng test --watch=false \
    --include=src/app/desktop/window-interaction.service.spec.ts \
    --include=src/app/desktop/window-manager.service.spec.ts \
    --include=src/app/desktop/desktop.component.spec.ts
  ```

  Expected: the service contract and existing route-shell/window-manager
  integration tests all pass.

### Task 3: Document, verify, and commit the refactor

**Files:**

- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-07-26-window-interaction-service.md`
- Review: every file changed by Tasks 1 and 2.

**Interfaces:**

- Produces: an updated Angular code map naming the interaction service as the
  algorithm owner.

- [x] **Step 1: Update the project code map**

  Add `frontend-ng/src/app/desktop/window-interaction.service.ts` to the
  Angular map and state that `DesktopComponent` coordinates DOM pointer
  lifecycles while the service owns interaction algorithms.

- [x] **Step 2: Format and inspect**

  Run Prettier on the changed TypeScript/spec files, then run:

  ```bash
  git diff --check
  git diff --stat
  git status --short
  ```

  Confirm the diff contains no markup/style changes and
  `.playwright-mcp/` remains the only unrelated untracked path.

- [x] **Step 3: Run the complete frontend confidence gate**

  Run:

  ```bash
  cd frontend-ng
  npm run test:ci
  npm run build
  ```

  Require zero test failures and a successful Angular production build.

- [x] **Step 4: Mark this plan complete and commit**

  Mark each checkbox only after its evidence exists, then commit the
  implementation, tests, plan, and `AGENTS.md` with:

  ```bash
  git commit -m "refactor(frontend): extract window interaction service"
  ```
