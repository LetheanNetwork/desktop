<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Shared Desktop Data Resource and Telemetry Design

## Goal

Create one truthful, typed lifecycle for desktop data and prove it through the
Telemetry application.

The first tranche standardises how an application represents demo, loading,
live, mixed, stale, and unavailable data without expanding into a general
bridge framework. Telemetry adopts the contract completely. Control, Files,
`SurfacePage`, and Go telemetry capabilities remain separate follow-up work.

## Current problem

The Angular desktop currently has three overlapping data-state vocabularies:

- `DesktopDataState` supports demo, loading, live, mixed, and unavailable;
- Files adds a separate stale state and preserves the last successful live
  result;
- `SurfaceDataState` uses fixture, loading, live, and offline.

The lifecycle behaviour is duplicated alongside those types. In particular:

- Files keeps its last successful provider result after a transient failure;
- Telemetry clears its sample and chart history, then returns to fixture
  values after a failed refresh;
- Control reconciles partial live sections with labelled demo sections;
- status, source, refresh time, retry, and stale presentation are not one
  reusable interface.

This makes connected failures visually inconsistent and makes each new live
application repeat the same state decisions.

## Compatibility contract

This design must preserve the following product decisions:

- explicit offline transport remains a deterministic, useful browser-demo
  mode and makes no Wails calls or event subscriptions;
- demo values are visibly labelled;
- connected data is never silently represented as live when it is a fixture,
  stale, missing, or unavailable;
- Telemetry keeps its current route, window behaviour, layout, charts,
  localisation, and bounded polling interval;
- missing native power data may retain the explicitly labelled demo power
  presentation, producing a mixed state rather than a false live reading;
- the existing `DesktopDataStateBadge`, Control views, Files state, and
  `SurfacePage` contract remain valid until their own migration tranches;
- no Go service, Wails binding, transport, NgRx store, desktop route, or design
  token changes are required;
- no raw backend error object, stack trace, or transport payload is rendered to
  the user.

## Approaches considered

### Shared resource contract plus one proving application — selected

Introduce a small immutable resource model and pure transition functions, then
adopt them in Telemetry. This establishes a tested lifecycle without forcing a
repository-wide migration.

### Add richer Telemetry sources first

Implementing token throughput, native power, accelerator use, or retained Go
history would make the window more useful, but it would deepen the current
state duplication. Those sources will fit more cleanly once the renderer has a
truthful lifecycle contract.

### Migrate every desktop application together

Converting Control, Files, `SurfacePage`, and Terminal in one change would
remove more duplication immediately, but it would mix applications with
different fallback and security policies. It is too broad for one reviewable
tranche.

## Shared resource model

Add `frontend-ng/src/app/desktop/desktop-data-resource.ts`.

The public contract is:

```ts
export type DesktopDataResourceMode = 'demo' | 'connected';

export type DesktopDataResourceState =
  | 'demo'
  | 'loading'
  | 'live'
  | 'mixed'
  | 'stale'
  | 'unavailable';

export interface DesktopDataStatus {
  readonly mode: DesktopDataResourceMode;
  readonly state: DesktopDataResourceState;
  readonly source: string;
  readonly updatedAt: number | null;
  readonly refreshing: boolean;
  readonly error: string | null;
  readonly canRetry: boolean;
}

export interface DesktopDataResource<T> extends DesktopDataStatus {
  readonly value: T | null;
}
```

`updatedAt` is the local receipt time of the most recent successful live
result. It is not described as a backend sample timestamp until the Go
contract supplies one. Demo resources use `updatedAt: null`.

`source` is short reader-facing provenance such as `Lethean demo fixture` or
`Local process runtime`. Applications supply localised source labels; the
shared resource does not infer them.

`refreshing` is independent of `state`. A resource may remain visibly live,
mixed, or stale while a background refresh runs. Initial connected loading has
no value and uses `state: 'loading'`.

`canRetry` is false for demo, loading, refreshing, and successful resources.
A rejected connected refresh sets it true once the active request has ended.

## Pure transitions

The same file exports pure functions. They do not inject Angular services,
read the clock, call Wails, schedule timers, mutate inputs, or log errors.

- `createDemoResource(value, source)` creates a ready demo resource.
- `createConnectedResource(source)` creates an empty loading resource.
- `beginDesktopDataRefresh(resource, now, staleAfterMs)` sets
  `refreshing: true`, clears the previous reader-facing error, and disables
  Retry. If an existing successful value has exceeded the stale threshold, it
  also marks that value stale.
- `resolveDesktopData(resource, value, state, source, now)` accepts only
  `live` or `mixed`, stores the successful value, clears the error, records
  `updatedAt`, ends refreshing, and disables Retry.
- `rejectDesktopData(resource, error)` ends refreshing. A connected resource
  with a previous successful value becomes stale and keeps that value; a
  resource without one becomes unavailable with `value: null`. Both rejected
  states enable Retry.

The caller maps unknown failures to stable, localised reader-facing text before
calling the rejection transition.

The transition API rejects impossible combinations in development and tests:

- demo mode cannot begin, resolve, or reject a live refresh;
- resolution and rejection require an active connected refresh;
- live, mixed, or stale states require a non-null value;
- unavailable requires a null value;
- a successful resolution cannot use demo, loading, stale, or unavailable as
  its result state.

## Shared status presenter

Add
`frontend-ng/src/app/desktop/desktop-data-status.view.ts`.

`DesktopDataStatusView` is a standalone, `OnPush` presentation component. It
receives `DesktopDataStatus` and emits a retry intent. It does not receive the
resource value and does not inject a bridge.

It renders:

- the state badge and its established Lethean variants;
- the source label;
- a localised last-received time when `updatedAt` is available;
- an unobtrusive refreshing indication while retaining current data;
- a stale or unavailable explanation;
- an accessible Retry button when `canRetry` is true and no refresh is active.

The existing `DesktopDataStateBadge` remains a compatibility component.
Control and Files are not mechanically adapted in this tranche.

## Telemetry view data

Add application-local
`frontend-ng/src/app/desktop/apps/telemetry/telemetry-view.models.ts` and
`telemetry-view-state.ts`.

`TelemetryViewData` contains:

- the current parsed process sample;
- bounded heap and power history;
- explicit provenance for the power panel;
- the formatted uptime and display values required by the existing template.

Pure Telemetry mapping functions:

- create a fresh deterministic demo view from the existing fixtures;
- convert a `ProcessTelemetry` response plus prior successful history into a
  new live view;
- cap every history series at the existing maximum;
- mark the result mixed when `wattsActive` is unavailable and the power panel
  deliberately uses its labelled demo series;
- never manufacture live token throughput, model identity, region, KV-cache,
  request rate, or native power values.

The view model keeps per-panel provenance, so a stale mixed resource still
shows that its power panel was demo-backed before the live refresh failed.

## Telemetry lifecycle

`TelemetryApp` owns one signal:

```ts
signal<DesktopDataResource<TelemetryViewData>>
```

It no longer owns separate sample, data-state, heap-history, and power-history
signals.

### Offline demo

On explicit offline transport:

1. initialise a demo resource from the deterministic Telemetry fixtures;
2. do not call `DesktopLiveDataService.telemetry()`;
3. do not create a polling timer;
4. render the existing useful charts with an explicit demo status and source.

### Connected startup

On connected transport:

1. initialise an empty connected resource;
2. enter the guarded refresh path immediately;
3. show loading placeholders rather than unlabelled fixture numbers;
4. start bounded polling only after initialisation.

### Successful refresh

On success:

1. parse the response through the existing `DesktopLiveDataService`;
2. build a fresh Telemetry view from the sample and previous successful
   history;
3. resolve to live when every displayed panel is live;
4. resolve to mixed when a displayed panel deliberately remains demo-backed;
5. record the local receipt time and clear the previous error.

### Failed refresh

On failure:

- before any successful result, transition to unavailable and show no
  connected fixture substitution;
- after a successful result, transition to stale while retaining the complete
  last successful view and chart history;
- keep panel provenance truthful;
- expose Retry through the status presenter;
- allow the next poll or manual retry to recover the same resource.

### Concurrency and teardown

One private refresh guard prevents overlapping Wails calls. A poll that fires
while a request is active is skipped rather than queued.

Manual Retry calls the same guarded refresh function. `ngOnDestroy` clears the
polling timer, and a late result after destruction must not update component
state.

The existing bounded poll remains the only update source. Push telemetry,
transport-reconnection orchestration, and retained Go history are deferred,
but a future event result can enter through the same successful transition.

## Data flow

```text
explicit offline transport
          |
          +----> deterministic Telemetry demo
          |                |
          |                v
          |      DesktopDataResource<TelemetryViewData>
          |
connected transport
          |
          v
DesktopLiveDataService.telemetry()
          |
          v
pure Telemetry view mapper
          |
          v
pure resource transition
          |
          +----> Telemetry panels
          |
          +----> DesktopDataStatusView ----> retry intent
```

## Error and truthfulness rules

- Demo mode never calls Wails.
- Loading connected mode does not display fixture numbers as if they were
  connected data.
- Mixed means at least one displayed section is live and at least one is
  explicitly demo-backed.
- Stale means a previous successful connected value is being retained after a
  failed or overdue refresh.
- Unavailable means no successful connected value exists.
- Reader-facing errors use stable localised copy; raw errors remain available
  only to diagnostics that already own them.
- A failed refresh never clears a previously successful history.
- A successful recovery clears stale/error presentation without resetting
  history.

## Testing

Use red-green Angular tests for each boundary.

### Resource transition tests

- demo initialisation;
- connected loading initialisation;
- live and mixed success;
- receipt timestamp recording;
- refresh-in-progress without hiding an existing value;
- age-based stale detection;
- first failure to unavailable;
- later failure to stale with value preservation;
- source and receipt timestamp preservation across rejection;
- Retry disabled during refresh, enabled after rejection, and cleared by
  recovery;
- stale recovery to live or mixed;
- impossible mode/state combinations rejected;
- input resources and values remain immutable.

### Status presenter tests

- state labels and variants;
- source and last-received presentation;
- refreshing, stale, and unavailable copy;
- Retry accessibility and emitted intent;
- no Retry action in demo mode or without an error.

### Telemetry mapping tests

- deterministic demo view;
- bounded history append;
- no mutation of previous history;
- mixed state when native power is absent;
- live state when native power is present;
- truthful panel provenance and formatting.

### Telemetry container tests

- no live call or timer in demo mode;
- connected loading does not render fixture values;
- initial success and polling;
- initial failure to unavailable;
- later failure preserves stale sample and charts;
- manual retry uses the same refresh path;
- overlapping refreshes are skipped;
- recovery clears stale state;
- timer and late-result cleanup on destroy;
- route and window integration remain unchanged.

Final verification runs:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-resource.spec.ts \
  --include=src/app/desktop/desktop-data-status.view.spec.ts \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts

cd ..
go tool wails3 task verify:frontend
git diff --check
```

## Acceptance criteria

The tranche is complete when:

- Telemetry uses one `DesktopDataResource<TelemetryViewData>` lifecycle;
- browser demo remains deterministic, visibly labelled, and Wails-free;
- connected startup contains no unlabelled fixture substitution;
- a first failure is unavailable;
- a later failure retains the last successful sample and history as stale;
- live, mixed, stale, and unavailable states show source, receipt time, and
  appropriate retry presentation;
- polling cannot overlap and is cleaned up on destruction;
- existing Telemetry layout, route, localisation, and desktop behaviour remain
  intact;
- focused tests and the frontend confidence gate pass.

## Deferred work

This tranche does not:

- add runner token throughput, model identity, request rate, queue depth,
  region, KV-cache use, or model uptime;
- implement native macOS, Windows, or Linux power helpers;
- add accelerator memory or utilisation;
- retain telemetry history in Go;
- add push events or transport-reconnection recovery;
- migrate Control, Files, Terminal, or `SurfacePage`;
- remove their compatibility state types;
- address frontend coverage or dependency security alerts.
