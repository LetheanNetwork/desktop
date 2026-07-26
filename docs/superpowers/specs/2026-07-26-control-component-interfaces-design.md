<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Control Component Interfaces Design

## Goal

Turn the 735-line Control application into a small Angular container with
separate, typed interfaces for Models, Runs, Power, System, and Settings.

This is the first app-level component pattern for Lethean Desktop. It must make
future visual and interaction work easier without removing features, changing
the desktop shell, or coupling presentation components to Wails.

## Compatibility contract

The current Control application remains the product contract during the
extraction:

- the Control launcher, route, rail order, icons, headings, cards, charts,
  tables, tabs, configuration fields, buttons, localisation, and demo values
  remain present;
- `win.sub` continues to select Models, Runs, Power, System, or Settings, and
  `win.systab` continues to select the System sub-view;
- `WindowManagerService` remains the source of navigation state;
- `DesktopLiveDataService` remains the only owner of Control's Wails method
  names and response parsing;
- explicit offline transport makes no live calls and retains the useful,
  visibly labelled browser demo;
- partial live reads continue to replace only the data they actually provide;
- the existing WebMCP tools retain their names, schemas, validation, and
  effects;
- no Go service, Wails binding, NgRx state, route, window behaviour, or
  production build path changes;
- placeholder controls such as Load model, New run, and Commit remain visible.
  Their component outputs establish future action seams but do not invent
  backend behaviour.

Models is the first acceptance surface. In this tranche, “polish” means a clear
typed component interface, `OnPush` rendering, signal inputs, accessible
labels, and focused tests. A later design pass can safely change its visual or
interaction details once this stable seam exists.

## Approaches considered

### Typed container and presentation views — selected

Keep `ControlApp` as the route-level container. Extract one standalone
presentation component for each rail section and give it a focused view model.
Pure mapping functions convert demo fixtures plus an optional live snapshot
into those view models.

This preserves the existing application while creating interfaces that can be
previewed and tested without Wails or the desktop shell.

### Convert Control into generic `SurfacePage` descriptors

This would remove more bespoke markup, but Control's nested tabs,
configuration controls, mixed live/demo data, and future actions exceed the
generic list-and-card surface contract. Expanding `SurfacePage` to cover them
would make that already broad abstraction harder to understand.

### Redesign each Control section during extraction

Combining structural extraction with a visual rewrite would make regressions
harder to locate and would obscure whether changed behaviour came from the new
component boundary or the redesign. Visual work follows one section at a time
after compatibility is established.

## Component boundaries

The route registry continues to lazy-load `ControlApp`. Its template owns only
the rail and selection of the active section.

Create an `apps/control/` directory with:

- `control-view.models.ts` — readonly view-model and action-intent types;
- `control-demo.data.ts` — typed Control fixtures currently embedded in the
  component;
- `control-view-state.ts` — pure formatting and demo/live reconciliation;
- `control-models.view.ts` — Models toolbar, metrics, chart, and table;
- `control-runs.view.ts` — benchmark toolbar, chart, and table;
- `control-power.view.ts` — power cards, chart, and explanatory copy;
- `control-system.view.ts` — System tabs, telemetry cards, process/daemon
  tables, and navigation output;
- `control-settings.view.ts` — configuration groups, flags, and Commit intent.

Each view is standalone, uses `ChangeDetectionStrategy.OnPush`, uses
function-based signal inputs and outputs available in Angular 22, and has a
`display: contents` host where required to preserve the existing layout and
global selectors.

Views do not inject `DesktopLiveDataService`, `WindowManagerService`, NgRx, or
the Wails runtime. They render their inputs and emit typed user intents.

`ControlApp` retains:

- the live-data lifecycle and `DesktopDataState`;
- the current `Win` and rail navigation;
- System tab delegation to `WindowManagerService`;
- WebMCP registration;
- future orchestration of Load, Run, process, and configuration actions.

## Typed view state

`ControlViewState` contains one readonly model per extracted view. Its fields
remain semantic rather than being pre-serialised custom-element attributes:

- metric values and labels;
- chart labels and readonly numeric samples;
- typed datatable columns and rows;
- configuration groups and flags;
- the current data state.

The leaf view serialises a chart or datatable payload only at the
`<lthn-chart>` or `<lthn-datatable>` boundary. JSON strings are no longer the
state shared between the container, WebMCP tools, and templates.

`createDemoControlViewState()` returns a fresh state derived from the existing
fixtures. `mergeControlLiveSnapshot(demo, snapshot)` is a pure function that
replaces only successful sections:

- model catalogue and benchmark history update Models and Runs;
- process telemetry updates uptime and System metrics;
- tracked processes replace only the process table fields the backend really
  supplies;
- curated settings replace configuration rows and flags;
- unsupported power, CPU history, daemon health, and model runtime fields stay
  demo-backed and keep the overall state labelled `Live + demo`.

The mapper does not call services, mutate its input, or invent missing live
values.

## Shared data-state presentation

Add a small standalone data-state badge component that receives
`DesktopDataState` and delegates to the existing label and variant helpers.
Control's five views reuse it instead of repeating badge markup.

Telemetry and Files may adopt the same presentation component in this tranche
if doing so is mechanical and their focused tests remain unchanged.
`SurfacePage` keeps its existing state model for now; reconciling stale live
rows and offline fixtures across the wider surface catalogue remains a
separate, explicit change rather than scope hidden inside Control.

## Development entrypoint

Add an npm `demo` script equivalent to the documented frontend development
command:

```text
ng serve --host 127.0.0.1 --port 9245 --hmr --poll 1000
```

The script does not choose a runtime mode. Developers open the explicit demo
URL when they want a backend-independent preview:

```text
http://127.0.0.1:9245/?lthn-offline=1#/w/control
```

The existing `npm start`, Wails development task, hash routing, and production
build remain unchanged.

## Data and event flow

```text
DesktopLiveDataService
          |
          v
ControlApp container ----> pure ControlViewState mapper
          |                           |
          |                           +----> demo + partial live view models
          v
active standalone Control view
          |
          +----> typed navigation/action intent ----> ControlApp
```

The browser demo path stops at the typed demo state and never enters
`DesktopLiveDataService.control()`.

## Testing

Use red-green Angular tests for each boundary:

- pure state tests preserve every demo metric, chart, column, row, setting, and
  flag while proving partial live reconciliation;
- each standalone view renders its supplied model and emits its typed intents
  without desktop or Wails providers;
- the Control container still performs no call in demo mode, maps partial live
  data, delegates rail/System navigation, and exposes the same WebMCP tools;
- the lazy route still resolves `ControlApp`;
- Telemetry and Files retain their current demo/live/failure tests if the
  shared badge is adopted;
- package-script contract coverage verifies the demo command without starting
  a second development server.

Final verification runs the focused Control tests, the frontend confidence
gate, the Angular production build, and `git diff --check`.

## Deferred work

This tranche does not implement model loading, benchmark execution, process
actions, configuration writes, host power sampling, general file operations,
or a new visual design. Those capabilities remain in `TODO.md` and can be
added behind the typed outputs and view models without rebuilding the Control
shell.
