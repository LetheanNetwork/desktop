<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Control Component Interfaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 735-line Control monolith with a route/data container and
five typed standalone presentation views while preserving every current UX
feature and demo/live behaviour.

**Architecture:** `ControlApp` remains the lazy route, Wails lifecycle,
navigation, and WebMCP owner. Pure functions reconcile typed demo data with a
partial `ControlLiveSnapshot`; Models, Runs, Power, System, and Settings render
readonly view models and emit typed intents without injecting application
services.

**Tech Stack:** Angular 22 standalone components, signal inputs/outputs,
`OnPush`, Vitest/TestBed, TypeScript 6, Lit custom elements, npm, Wails 3.

## Global Constraints

- Execute inline on `main`; do not use sub-agents or create a worktree.
- Preserve all Control rail entries, tabs, headings, cards, charts, tables,
  buttons, copy, localisation ids, WebMCP tools, and placeholder controls.
- Preserve `win.sub`, `win.systab`, `WindowManagerService`, hash routes, NgRx,
  Wails bindings, and production output.
- Explicit offline transport makes no Wails calls and shows labelled demo
  fixtures; partial live data replaces only truthful fields.
- Use British English and retain EUPL-1.2 headers.
- Do not add backend contracts, SSR, hydration, another frontend framework, or
  feature gates.
- Write the failing test before each production change.
- Leave the user-owned `.playwright-mcp/` directory untouched.

---

### Task 1: Typed Control view state and pure reconciliation

**Files:**

- Create:
  `frontend/src/app/desktop/apps/control/control-view.models.ts`
- Create: `frontend/src/app/desktop/apps/control/control-demo.data.ts`
- Create: `frontend/src/app/desktop/apps/control/control-view-state.ts`
- Test:
  `frontend/src/app/desktop/apps/control/control-view-state.spec.ts`

**Interfaces:**

- Consumes: `ControlLiveSnapshot` from `desktop-live-data.service.ts` and
  `DesktopDataState` from `desktop-data-state.ts`.
- Produces:
  `createDemoControlViewState(): ControlViewState` and
  `mergeControlLiveSnapshot(snapshot: ControlLiveSnapshot):
ControlViewState`.

- [ ] **Step 1: Write the failing pure-state tests**

Create `control-view-state.spec.ts` with these contracts:

```ts
import {
  createDemoControlViewState,
  mergeControlLiveSnapshot,
} from "./control-view-state";

describe("Control view state", () => {
  it("preserves the complete labelled Control demo", () => {
    const state = createDemoControlViewState();

    expect(state.dataState).toBe("demo");
    expect(state.models.metrics.map(({ value }) => value)).toEqual([
      "34.2",
      "18.4 GB",
      "128",
      "6d 4h",
    ]);
    expect(state.models.rows).toHaveLength(6);
    expect(state.runs.rows).toHaveLength(4);
    expect(state.power.samples).toHaveLength(12);
    expect(state.system.processRows).toHaveLength(6);
    expect(state.system.daemonRows).toHaveLength(4);
    expect(state.settings.groups.map(({ name }) => name)).toEqual([
      "Server",
      "Models",
    ]);
    expect(state.settings.flags).toHaveLength(3);
  });

  it("replaces only successful live sections", () => {
    const state = mergeControlLiveSnapshot({
      models: [
        {
          name: "gemma.gguf",
          path: "/tmp/gemma.gguf",
          sizeBytes: 2_147_483_648,
          isDirectory: false,
        },
      ],
      processes: [
        {
          id: "build-1",
          command: "npm run build",
          status: "running",
          exitCode: 0,
        },
      ],
      unavailable: ["telemetry", "benchmarkRuns", "settings"],
    });

    expect(state.dataState).toBe("mixed");
    expect(state.models.rows).toEqual([
      {
        name: "gemma.gguf",
        size: "2 GB",
        source: "local file",
        status: "available",
      },
    ]);
    expect(state.system.processColumns.map(({ key }) => key)).toEqual([
      "command",
      "id",
      "state",
      "exit",
    ]);
    expect(state.power.metrics[0].value).toBe("196 W");
    expect(state.settings.groups[0].name).toBe("Server");
  });
});
```

- [ ] **Step 2: Run the spec and observe the missing-module failure**

Run:

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/control/control-view-state.spec.ts
```

Expected: FAIL because `control-view-state.ts` does not exist.

- [ ] **Step 3: Define the semantic view-model types**

In `control-view.models.ts`, define readonly types with no Angular or Wails
imports other than `DesktopDataState`:

```ts
import type { DesktopDataState } from "../../desktop-data-state";

export type ControlTableCell = string | number;
export type ControlTableRow = Readonly<Record<string, ControlTableCell>>;
export type ControlColumnType = "num" | "mono" | "status";

export interface ControlTableColumn {
  readonly key: string;
  readonly label: string;
  readonly type?: ControlColumnType;
}

export interface ControlMetric {
  readonly value: string;
  readonly label: string;
}

export interface ControlChart {
  readonly title: string;
  readonly caption: string;
  readonly samples: readonly number[];
}

export interface ControlModelsViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly chart: ControlChart;
  readonly columns: readonly ControlTableColumn[];
  readonly rows: readonly ControlTableRow[];
}

export interface ControlRunsViewModel {
  readonly chart: ControlChart;
  readonly columns: readonly ControlTableColumn[];
  readonly rows: readonly ControlTableRow[];
}

export interface ControlPowerViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly samples: readonly number[];
}

export type ControlSystemTab = "overview" | "processes" | "daemons";

export interface ControlSystemViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly cpuSamples: readonly number[];
  readonly processColumns: readonly ControlTableColumn[];
  readonly processRows: readonly ControlTableRow[];
  readonly daemonColumns: readonly ControlTableColumn[];
  readonly daemonRows: readonly ControlTableRow[];
}

export interface ControlSettingRow {
  readonly key: string;
  readonly value: string;
  readonly source: string;
}

export interface ControlSettingGroup {
  readonly name: string;
  readonly rows: readonly ControlSettingRow[];
}

export interface ControlSettingFlag {
  readonly key: string;
  readonly on: boolean;
  readonly source: string;
}

export interface ControlSettingsViewModel {
  readonly groups: readonly ControlSettingGroup[];
  readonly flags: readonly ControlSettingFlag[];
}

export interface ControlViewState {
  readonly dataState: DesktopDataState;
  readonly models: ControlModelsViewModel;
  readonly runs: ControlRunsViewModel;
  readonly power: ControlPowerViewModel;
  readonly system: ControlSystemViewModel;
  readonly settings: ControlSettingsViewModel;
}

export type ControlActionIntent =
  | { readonly kind: "load-model" }
  | { readonly kind: "new-run" }
  | { readonly kind: "commit-settings" };
```

- [ ] **Step 4: Move the existing fixtures into typed data**

In `control-demo.data.ts`, export
`CONTROL_DEMO_VIEW_STATE: ControlViewState`. Copy every current Control
literal from `control.app.ts` without changing values:

- four Models metrics, the throughput title/caption/samples, five model
  columns, and six model rows;
- Runs chart, five columns, and four rows;
- three power metrics and the existing `TELEMETRY.watts`;
- four System metrics, `TELEMETRY.throughput`, five process columns/six rows,
  five daemon columns/four rows;
- two configuration groups and three flags.

Set `dataState: 'demo'` and use the current `$localize` ids for every label.
Use `satisfies ControlViewState` so malformed fixtures fail compilation.

- [ ] **Step 5: Implement the pure mapper**

In `control-view-state.ts`, export:

```ts
export function createDemoControlViewState(): ControlViewState {
  return {
    ...CONTROL_DEMO_VIEW_STATE,
    models: { ...CONTROL_DEMO_VIEW_STATE.models },
    runs: { ...CONTROL_DEMO_VIEW_STATE.runs },
    power: { ...CONTROL_DEMO_VIEW_STATE.power },
    system: { ...CONTROL_DEMO_VIEW_STATE.system },
    settings: { ...CONTROL_DEMO_VIEW_STATE.settings },
  };
}

export function mergeControlLiveSnapshot(
  snapshot: ControlLiveSnapshot,
): ControlViewState {
  const demo = createDemoControlViewState();
  return {
    ...demo,
    dataState: "mixed",
    models: mergeModels(demo.models, snapshot),
    runs: mergeRuns(demo.runs, snapshot),
    system: mergeSystem(demo.system, snapshot),
    settings: mergeSettings(demo.settings, snapshot),
  };
}
```

Move `formatBytes`, `formatUptime`, `formatMegabytes`, and `benchmarkTime`
from `control.app.ts` into this file. Implement `mergeModels`, `mergeRuns`,
`mergeSystem`, and `mergeSettings` with the exact mapping currently performed
by `applyLiveSnapshot`; leave Power, CPU history, daemon rows, and unsupported
model runtime metrics from the demo state.

- [ ] **Step 6: Run the pure-state spec**

Run the Task 1 command again.

Expected: 2 tests PASS.

- [ ] **Step 7: Commit the typed state**

```bash
git add frontend/src/app/desktop/apps/control/
git commit -m "refactor(frontend): model typed Control view state"
```

---

### Task 2: Shared desktop data-state badge

**Files:**

- Create:
  `frontend/src/app/desktop/desktop-data-state-badge.ts`
- Test:
  `frontend/src/app/desktop/desktop-data-state-badge.spec.ts`
- Modify: `frontend/src/app/desktop/apps/telemetry.app.ts`
- Modify: `frontend/src/app/desktop/apps/files.app.ts`
- Test: `frontend/src/app/desktop/apps/telemetry.app.spec.ts`
- Test: `frontend/src/app/desktop/apps/files.app.spec.ts`

**Interfaces:**

- Consumes: `DesktopDataState`.
- Produces: `<lthn-desktop-data-state [state]="state" />`.

- [ ] **Step 1: Write the failing badge test**

```ts
import { TestBed } from "@angular/core/testing";
import { DesktopDataStateBadge } from "./desktop-data-state-badge";

it("renders the canonical label, variant, and machine-readable state", () => {
  const fixture = TestBed.createComponent(DesktopDataStateBadge);
  fixture.componentRef.setInput("state", "mixed");
  fixture.detectChanges();

  const badge = fixture.nativeElement.querySelector("lthn-badge");
  expect(badge.textContent).toContain("Live + demo");
  expect(badge.getAttribute("variant")).toBe("warn");
  expect(badge.dataset.dataState).toBe("mixed");
});
```

- [ ] **Step 2: Run the spec and observe the missing-component failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts
```

Expected: FAIL because `DesktopDataStateBadge` does not exist.

- [ ] **Step 3: Implement the badge**

```ts
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
} from "@angular/core";
import {
  type DesktopDataState,
  desktopDataStateLabel,
  desktopDataStateVariant,
} from "./desktop-data-state";

@Component({
  selector: "lthn-desktop-data-state",
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: "display: contents" },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <lthn-badge [attr.variant]="variant()" [attr.data-data-state]="state()">
      {{ label() }}
    </lthn-badge>
  `,
})
export class DesktopDataStateBadge {
  readonly state = input.required<DesktopDataState>();
  readonly label = computed(() => desktopDataStateLabel(this.state()));
  readonly variant = computed(() => desktopDataStateVariant(this.state()));
}
```

- [ ] **Step 4: Replace duplicate badge markup mechanically**

Import `DesktopDataStateBadge` in Telemetry and Files, add it to each
component's `imports`, replace the `<lthn-badge>` block with:

```html
<lthn-desktop-data-state [state]="dataState()" />
```

Remove only the now-unused `dataStateLabel`, `dataStateVariant`, and helper
imports. Do not change polling, fallback, navigation, or visible copy.

- [ ] **Step 5: Run badge, Telemetry, and Files specs**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/desktop/apps/files.app.spec.ts
```

Expected: all specs PASS.

- [ ] **Step 6: Commit the badge**

```bash
git add frontend/src/app/desktop/desktop-data-state-badge*
git add frontend/src/app/desktop/apps/telemetry.app.ts
git add frontend/src/app/desktop/apps/files.app.ts
git commit -m "refactor(frontend): share desktop data-state badge"
```

---

### Task 3: Models and Runs presentation interfaces

**Files:**

- Create:
  `frontend/src/app/desktop/apps/control/control-models.view.ts`
- Create:
  `frontend/src/app/desktop/apps/control/control-runs.view.ts`
- Test:
  `frontend/src/app/desktop/apps/control/control-primary-views.spec.ts`

**Interfaces:**

- Consumes: `DesktopDataState`, `ControlModelsViewModel`,
  `ControlRunsViewModel`.
- Produces: `ControlModelsView.loadModel: OutputEmitterRef<void>` and
  `ControlRunsView.newRun: OutputEmitterRef<void>`.

- [ ] **Step 1: Write failing component-interface tests**

Create a TestBed spec that imports the typed demo state and verifies:

```ts
it("renders Models from its typed input and emits Load model", () => {
  const state = createDemoControlViewState();
  const fixture = TestBed.createComponent(ControlModelsView);
  fixture.componentRef.setInput("dataState", state.dataState);
  fixture.componentRef.setInput("model", state.models);
  const emitted = vi.fn();
  fixture.componentInstance.loadModel.subscribe(emitted);
  fixture.detectChanges();

  expect(fixture.nativeElement.textContent).toContain("Local models");
  expect(fixture.nativeElement.querySelectorAll("lthn-stat")).toHaveLength(4);
  expect(
    JSON.parse(
      fixture.nativeElement
        .querySelector("lthn-datatable")
        .getAttribute("rows"),
    ),
  ).toHaveLength(6);
  fixture.nativeElement.querySelector("button.nbtn").click();
  expect(emitted).toHaveBeenCalledOnce();
});

it("renders Runs from its typed input and emits New run", () => {
  const state = createDemoControlViewState();
  const fixture = TestBed.createComponent(ControlRunsView);
  fixture.componentRef.setInput("dataState", state.dataState);
  fixture.componentRef.setInput("model", state.runs);
  const emitted = vi.fn();
  fixture.componentInstance.newRun.subscribe(emitted);
  fixture.detectChanges();

  expect(fixture.nativeElement.textContent).toContain("Benchmark runs");
  fixture.nativeElement.querySelector("button.nbtn").click();
  expect(emitted).toHaveBeenCalledOnce();
});
```

- [ ] **Step 2: Run the spec and observe missing-component failures**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/control/control-primary-views.spec.ts
```

Expected: FAIL because both view components are absent.

- [ ] **Step 3: Implement `ControlModelsView`**

Use `input.required`, `output`, `computed`, `OnPush`,
`CUSTOM_ELEMENTS_SCHEMA`, and `DesktopDataStateBadge`:

```ts
@Component({
  selector: "lthn-control-models-view",
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: "display: contents" },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="Local model view heading@@control.models.heading">
        Local models
      </h1>
      <lthn-desktop-data-state [state]="dataState()" />
      <span class="miniseg">
        <span
          class="on"
          i18n="Running model filter@@control.models.filter.running"
          >Running</span
        >
        <span i18n="All models filter@@control.models.filter.all">All</span>
      </span>
      <button class="nbtn" (click)="loadModel.emit()">
        <lthn-icon name="plus" size="10"></lthn-icon>
        <span i18n="Load model action@@control.models.load">Load model</span>
      </button>
    </div>
    <div class="tiles">
      @for (metric of model().metrics; track metric.label) {
        <lthn-card pad="11">
          <lthn-stat
            [attr.value]="metric.value"
            [attr.label]="metric.label"
            mono
          />
        </lthn-card>
      }
    </div>
    <div class="panel">
      <div class="ph">
        <b>{{ model().chart.title }}</b>
        <span>{{ model().chart.caption }}</span>
      </div>
      <lthn-chart type="area" [attr.data]="samplesJson()" height="90" />
    </div>
    <lthn-datatable
      selectable
      [attr.columns]="columnsJson()"
      [attr.rows]="rowsJson()"
    />
  `,
})
export class ControlModelsView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlModelsViewModel>();
  readonly loadModel = output<void>();
  readonly columnsJson = computed(() => JSON.stringify(this.model().columns));
  readonly rowsJson = computed(() => JSON.stringify(this.model().rows));
  readonly samplesJson = computed(() =>
    JSON.stringify(this.model().chart.samples),
  );
}
```

Copy the current Models DOM exactly. Replace four separate signal reads with
`model().metrics[0..3]`, chart fields with `model().chart`, and table
attributes with the three computed JSON values. Bind Load model to
`loadModel.emit()`.

- [ ] **Step 4: Implement `ControlRunsView`**

Use the same component conventions. Copy the existing Runs DOM exactly, bind
the typed chart/table model, serialise only at custom-element attributes, and
bind New run to `newRun.emit()`.

- [ ] **Step 5: Run the primary-view spec**

Run the Task 3 test command again.

Expected: 2 tests PASS.

- [ ] **Step 6: Commit the primary views**

```bash
git add frontend/src/app/desktop/apps/control/control-models.view.ts
git add frontend/src/app/desktop/apps/control/control-runs.view.ts
git add frontend/src/app/desktop/apps/control/control-primary-views.spec.ts
git commit -m "refactor(frontend): extract Control model interfaces"
```

---

### Task 4: Power, System, and Settings presentation interfaces

**Files:**

- Create:
  `frontend/src/app/desktop/apps/control/control-power.view.ts`
- Create:
  `frontend/src/app/desktop/apps/control/control-system.view.ts`
- Create:
  `frontend/src/app/desktop/apps/control/control-settings.view.ts`
- Test:
  `frontend/src/app/desktop/apps/control/control-secondary-views.spec.ts`

**Interfaces:**

- Consumes: `DesktopDataState`, `ControlPowerViewModel`,
  `ControlSystemViewModel`, `ControlSettingsViewModel`, and
  `ControlSystemTab`.
- Produces:
  `ControlSystemView.tabChange: OutputEmitterRef<ControlSystemTab>` and
  `ControlSettingsView.commit: OutputEmitterRef<void>`.

- [ ] **Step 1: Write failing secondary-view tests**

Test the three views independently using the demo state:

```ts
it("keeps the complete Power prototype", () => {
  const state = createDemoControlViewState();
  const fixture = TestBed.createComponent(ControlPowerView);
  fixture.componentRef.setInput("dataState", state.dataState);
  fixture.componentRef.setInput("model", state.power);
  fixture.detectChanges();

  const text = fixture.nativeElement.textContent ?? "";
  expect(text).toContain("Power");
  expect(text).toContain("≈ a small fridge");
  expect(fixture.nativeElement.querySelectorAll("lthn-stat")).toHaveLength(3);
});

it("emits a typed System tab request", () => {
  const state = createDemoControlViewState();
  const fixture = TestBed.createComponent(ControlSystemView);
  fixture.componentRef.setInput("dataState", state.dataState);
  fixture.componentRef.setInput("model", state.system);
  fixture.componentRef.setInput("activeTab", "overview");
  const emitted = vi.fn();
  fixture.componentInstance.tabChange.subscribe(emitted);
  fixture.detectChanges();

  fixture.nativeElement.querySelectorAll("button.systab")[1].click();
  expect(emitted).toHaveBeenCalledWith("processes");
});

it("renders settings and emits Commit", () => {
  const state = createDemoControlViewState();
  const fixture = TestBed.createComponent(ControlSettingsView);
  fixture.componentRef.setInput("dataState", state.dataState);
  fixture.componentRef.setInput("model", state.settings);
  const emitted = vi.fn();
  fixture.componentInstance.commit.subscribe(emitted);
  fixture.detectChanges();

  expect(fixture.nativeElement.textContent).toContain("features.lethernet");
  fixture.nativeElement.querySelector("button.nbtn").click();
  expect(emitted).toHaveBeenCalledOnce();
});
```

- [ ] **Step 2: Run the spec and observe missing-component failures**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts
```

Expected: FAIL because the three views are absent.

- [ ] **Step 3: Implement the Power view**

Copy the existing Power template exactly. Read its three metrics and chart
samples from `ControlPowerViewModel`, render the shared data-state badge, and
serialise samples in a computed signal.

- [ ] **Step 4: Implement the System view**

Copy the System toolbar, tabs, overview, process, and daemon markup. Define:

```ts
readonly dataState = input.required<DesktopDataState>();
readonly model = input.required<ControlSystemViewModel>();
readonly activeTab = input.required<ControlSystemTab>();
readonly tabChange = output<ControlSystemTab>();
readonly processColumnsJson = computed(() => JSON.stringify(this.model().processColumns));
readonly processRowsJson = computed(() => JSON.stringify(this.model().processRows));
readonly daemonColumnsJson = computed(() => JSON.stringify(this.model().daemonColumns));
readonly daemonRowsJson = computed(() => JSON.stringify(this.model().daemonRows));
readonly cpuSamplesJson = computed(() => JSON.stringify(this.model().cpuSamples));
```

Use the current three localised tab labels and emit their typed ids. Preserve
the explicit demo CPU caption and truthful process help.

- [ ] **Step 5: Implement the Settings view**

Copy the existing configuration DOM and `LthnToggleOnDirective` into
`control-settings.view.ts`. Change row fields from `k`/`v`/`src` to
`key`/`value`/`source`, move `sourceLabel` into this view, and bind Commit to
`commit.emit()`.

- [ ] **Step 6: Run the secondary-view spec**

Run the Task 4 command again.

Expected: 3 tests PASS.

- [ ] **Step 7: Commit the secondary views**

```bash
git add frontend/src/app/desktop/apps/control/control-power.view.ts
git add frontend/src/app/desktop/apps/control/control-system.view.ts
git add frontend/src/app/desktop/apps/control/control-settings.view.ts
git add frontend/src/app/desktop/apps/control/control-secondary-views.spec.ts
git commit -m "refactor(frontend): extract remaining Control interfaces"
```

---

### Task 5: Reduce `ControlApp` to the route/data container

**Files:**

- Modify: `frontend/src/app/desktop/apps/control.app.ts`
- Modify: `frontend/src/app/desktop/apps/control.app.spec.ts`
- Modify: `frontend/src/app/desktop/apps/app-mcp.spec.ts`

**Interfaces:**

- Consumes: every view and state interface from Tasks 1–4.
- Produces: the unchanged `ControlApp implements AppView` lazy-route
  contract.

- [ ] **Step 1: Add a failing container-boundary assertion**

In `control.app.spec.ts`, add:

```ts
it("delegates each rail section to its standalone view", async () => {
  const expectations = [
    ["models", "lthn-control-models-view"],
    ["runs", "lthn-control-runs-view"],
    ["power", "lthn-control-power-view"],
    ["system", "lthn-control-system-view"],
    ["settings", "lthn-control-settings-view"],
  ] as const;

  for (const [sub, selector] of expectations) {
    const fixture = await create({ ...controlWin, sub });
    expect(fixture.nativeElement.querySelector(selector)).not.toBeNull();
    fixture.destroy();
  }
});

it("remains the lazy component registered for Control", async () => {
  await expect(APP_REGISTRY["control"]()).resolves.toBe(ControlApp);
});
```

- [ ] **Step 2: Run existing Control and WebMCP specs**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/app-mcp.spec.ts
```

Expected: the new standalone-view assertion FAILS while existing behaviour
tests still pass.

- [ ] **Step 3: Replace the monolithic template**

Keep the rail markup and replace the inline section bodies with Angular 22
control flow:

```html
<div class="appbody">
  @switch (win.sub || 'models') { @case ('models') {
  <lthn-control-models-view
    [dataState]="viewState().dataState"
    [model]="viewState().models"
    (loadModel)="handleAction({ kind: 'load-model' })"
  />
  } @case ('runs') {
  <lthn-control-runs-view
    [dataState]="viewState().dataState"
    [model]="viewState().runs"
    (newRun)="handleAction({ kind: 'new-run' })"
  />
  } @case ('power') {
  <lthn-control-power-view
    [dataState]="viewState().dataState"
    [model]="viewState().power"
  />
  } @case ('system') {
  <lthn-control-system-view
    [dataState]="viewState().dataState"
    [model]="viewState().system"
    [activeTab]="systemTab()"
    (tabChange)="wm.setSysTab(win.id, $event)"
  />
  } @case ('settings') {
  <lthn-control-settings-view
    [dataState]="viewState().dataState"
    [model]="viewState().settings"
    (commit)="handleAction({ kind: 'commit-settings' })"
  />
  } }
</div>
```

Import all five views. Keep `CommonModule` only if the rail still uses
`*ngFor`; otherwise use `@for` and remove it.

- [ ] **Step 4: Replace mutable section signals with one state signal**

Define:

```ts
readonly viewState = signal<ControlViewState>({
  ...createDemoControlViewState(),
  dataState: this.liveData.mode() === 'demo' ? 'demo' : 'loading',
});

systemTab(): ControlSystemTab {
  const tab = this.win.systab;
  return tab === 'processes' || tab === 'daemons' ? tab : 'overview';
}
```

In `refresh`, set `loading`, then:

```ts
try {
  this.viewState.set(mergeControlLiveSnapshot(await this.liveData.control()));
} catch {
  this.viewState.set({
    ...createDemoControlViewState(),
    dataState: "unavailable",
  });
}
```

Delete `applyLiveSnapshot`, formatting helpers, inline fixture signals, and
the toggle directive now owned by Settings.

Keep a deliberately empty typed action seam:

```ts
handleAction(_intent: ControlActionIntent): void {
  // Existing placeholder controls intentionally have no backend action yet.
}
```

- [ ] **Step 5: Preserve WebMCP against typed rows**

Change only `control_read_state`'s model source:

```ts
models: this.viewState().models.rows,
```

Keep tool names, schemas, section validation, `WindowManagerService`
delegation, and abort cleanup unchanged.

- [ ] **Step 6: Run Control, WebMCP, registry, and state specs**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/control/control-view-state.spec.ts \
  --include=src/app/desktop/apps/control/control-primary-views.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/app-mcp.spec.ts
```

Expected: all focused specs PASS and the lazy registry still resolves
`ControlApp`.

- [ ] **Step 7: Commit the container extraction**

```bash
git add frontend/src/app/desktop/apps/control.app.ts
git add frontend/src/app/desktop/apps/control.app.spec.ts
git add frontend/src/app/desktop/apps/app-mcp.spec.ts
git commit -m "refactor(frontend): componentise the Control app"
```

---

### Task 6: Deterministic browser demo entrypoint

**Files:**

- Modify: `frontend/package.json`
- Modify: `scripts/verify-frontend-convergence.test.mjs`
- Modify: `AGENTS.md`

**Interfaces:**

- Produces: `npm run demo`.

- [ ] **Step 1: Write the failing package-script contract**

Add to `verify-frontend-convergence.test.mjs`:

```js
test("frontend exposes the documented deterministic demo server", async () => {
  const packageJSON = JSON.parse(await read("frontend/package.json"));
  assert.equal(
    packageJSON.scripts.demo,
    "ng serve --host 127.0.0.1 --port 9245 --hmr --poll 1000",
  );
});
```

- [ ] **Step 2: Run the contract and observe failure**

```bash
node --test scripts/verify-frontend-convergence.test.mjs
```

Expected: FAIL because `scripts.demo` is undefined.

- [ ] **Step 3: Add the npm command**

Add this exact entry beside `start`:

```json
"demo": "ng serve --host 127.0.0.1 --port 9245 --hmr --poll 1000"
```

Do not add a package or modify `package-lock.json`.

- [ ] **Step 4: Update the developer command**

In `AGENTS.md`, make `npm run demo` the first frontend-only development
command, retain the expanded `npm start -- ...` form as its equivalent, and
keep the explicit `?lthn-offline=1` URL examples.

- [ ] **Step 5: Run the contract suite**

```bash
cd frontend
npm run test:contracts
```

Expected: all Node contract tests PASS.

- [ ] **Step 6: Commit the entrypoint**

```bash
git add frontend/package.json scripts/verify-frontend-convergence.test.mjs AGENTS.md
git commit -m "chore(frontend): add deterministic demo command"
```

---

### Task 7: Final verification and handoff

**Files:**

- Verify all modified files.

**Interfaces:**

- Consumes: Tasks 1–6.
- Produces: a verified `main` commit sequence ready to push.

- [ ] **Step 1: Format the touched frontend files**

```bash
cd frontend
npx prettier --write \
  package.json \
  src/app/desktop/desktop-data-state-badge.ts \
  src/app/desktop/desktop-data-state-badge.spec.ts \
  src/app/desktop/apps/control.app.ts \
  src/app/desktop/apps/control.app.spec.ts \
  src/app/desktop/apps/app-mcp.spec.ts \
  src/app/desktop/apps/telemetry.app.ts \
  src/app/desktop/apps/files.app.ts \
  src/app/desktop/apps/control/*.ts
```

- [ ] **Step 2: Run the complete focused Angular slice**

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts \
  --include=src/app/desktop/apps/control/control-view-state.spec.ts \
  --include=src/app/desktop/apps/control/control-primary-views.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/app-mcp.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/desktop/apps/files.app.spec.ts
```

Expected: every selected Vitest spec passes with zero failures.

- [ ] **Step 3: Run the frontend confidence gate**

```bash
npm run verify
```

Expected: capability audit, Node contracts, Angular CI tests, production
build, and embedded-build verification all pass. Record fresh coverage
honestly; the existing 70% target is not an enforced green threshold.

- [ ] **Step 4: Check repository hygiene**

```bash
cd ..
git diff --check
git status --short
git log --oneline --decorate -8
```

Expected: no whitespace errors, no generated binding/dist changes, and only
the pre-existing untracked `.playwright-mcp/` directory outside the committed
work.

- [ ] **Step 5: Report the component boundaries and verification**

Report:

- the new five-view Control structure;
- typed demo/live mapping and unchanged WebMCP/navigation ownership;
- shared data-state badge adoption;
- `npm run demo` and the offline preview URL;
- focused and complete verification counts;
- each implementation commit and whether `main` was pushed.
