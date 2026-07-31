<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Reactive Desktop Controls Synchronisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronise Settings and Control through one revisioned NgRx/RxJS state using the existing Wails WebSocket event path when connected and a versioned localStorage provider in explicit offline demo mode.

**Architecture:** `appconfig.Service` emits one bounded Core event after each durable batch commit; `pkg/desktop` forwards it as `lthn:desktop-controls:changed`. Angular validates connected events or offline storage notifications, reconciles them in `DesktopControlsEffects`, protects dirty drafts, and projects committed NgRx snapshots into `PreferencesService` signals.

**Tech Stack:** Go 1.26, CoreGO ACTION and `io.Medium`, Wails 3 events/WebSocket transport, Angular 22, NgRx 21, RxJS 7.8, Vitest, browser localStorage.

## Global Constraints

- `frontend/` remains the only Angular 22 product frontend; keep CSR and hash routing.
- NgRx/RxJS remains the cross-component and transport state boundary; signals are view adapters.
- Connected persistence flows only through the registered CoreGO config `io.Medium`; browser storage is never connected authority.
- Explicit offline mode opens no WebSocket, registers no Wails event listener, and makes no Wails binding call.
- Store only curated scalar demo controls in localStorage; do not introduce IndexedDB for this slice.
- Preserve dirty drafts on external events and reconnects until explicit Reload or Apply.
- Event and storage envelopes are bounded, exact, and contain no paths, credentials, environment data, commands, or arbitrary event names.
- Use British English, EUPL-1.2 headers, Good/Bad/Ugly Go tests, and colocated Angular Vitest specs.
- Work only on `feat/reactive-desktop-controls-sync`; do not push or modify a remote.

---

### Task 1: Add revisioned appconfig Core events

**Files:**

- Create: `go/pkg/appconfig/events.go`
- Create: `go/pkg/appconfig/events_test.go`
- Modify: `go/pkg/appconfig/service.go`
- Modify: `go/pkg/appconfig/service_test.go`
- Modify: `go/pkg/appconfig/service_example_test.go`

**Interfaces:**

- Consumes: `core.Core.ACTION`, the existing `Service.mu`, and successful `config.Service.Commit`.
- Produces: `appconfig.Event`, `appconfig.Subscribe(*core.Core, func(*core.Core, Event))`, and a `revision` field in every successful Settings snapshot.

- [ ] **Step 1: Write failing event and revision tests**

Add tests which subscribe before `SetMany`, assert one event after a successful batch, assert `event.Revision == snapshot["revision"]`, and prove no new event is emitted for invalid or commit-failed batches:

```go
events := []appconfig.Event{}
appconfig.Subscribe(c, func(_ *core.Core, event appconfig.Event) {
    events = append(events, event)
})

result := svc.SetMany([]appconfig.Change{{
    Key: "desktop.theme.interface", Value: "light",
}})

core.RequireTrue(t, result.OK, result.Error())
snapshot := result.Value.(map[string]any)
core.RequireTrue(t, len(events) == 1)
core.AssertEqual(t, snapshot["revision"], events[0].Revision)
core.AssertEqual(t, []string{"desktop.theme.interface"}, events[0].Keys)
core.AssertNotEqual(t, "", events[0].At)
```

Add nil Core/nil listener tests and an example which compiles `Subscribe`.

- [ ] **Step 2: Run the focused Go tests and confirm the red state**

Run:

```bash
go test ./go/pkg/appconfig -count=1
```

Expected: compilation fails because `appconfig.Event`, `Subscribe`, and snapshot revision do not exist.

- [ ] **Step 3: Implement the typed Core event**

Create `events.go`:

```go
type Event struct {
    Revision string   `json:"revision"`
    Keys     []string `json:"keys"`
    At       string   `json:"at"`
}

func Subscribe(c *core.Core, fn func(*core.Core, Event)) {
    if c == nil || fn == nil { return }
    c.RegisterAction(func(c *core.Core, message core.Message) core.Result {
        if event, ok := message.(Event); ok { fn(c, event) }
        return core.Ok(nil)
    })
}
```

Add `revision atomic.Uint64` to `Service`. Include `revisionString()` in `Settings`, increment only after `Commit` and live application, collect a fresh copy of the prepared keys, and publish:

```go
revision := strconv.FormatUint(s.revision.Add(1), 10)
_ = s.core.ACTION(Event{
    Revision: revision,
    Keys: keys,
    At: core.Now().UTC().Format(core.RFC3339Nano),
})
return s.Settings()
```

Empty batches continue to return `Settings` without incrementing or emitting.

- [ ] **Step 4: Run focused tests, race tests, formatting, and vet**

Run:

```bash
gofmt -w go/pkg/appconfig/events.go go/pkg/appconfig/events_test.go \
  go/pkg/appconfig/service.go go/pkg/appconfig/service_test.go \
  go/pkg/appconfig/service_example_test.go
go test ./go/pkg/appconfig -count=1
go test -race ./go/pkg/appconfig -count=1
go vet ./go/pkg/appconfig
```

Expected: all pass.

- [ ] **Step 5: Commit the backend event contract**

```bash
git add go/pkg/appconfig
git commit -m "feat(appconfig): emit committed control changes"
```

### Task 2: Forward appconfig events through the desktop host

**Files:**

- Create: `go/pkg/desktop/appconfig_events.go`
- Create: `go/pkg/desktop/appconfig_events_test.go`
- Modify: `go/pkg/desktop/desktop.go`

**Interfaces:**

- Consumes: `appconfig.Subscribe` from Task 1 and `emitCoreEvent`.
- Produces: the fixed Wails event `lthn:desktop-controls:changed`.

- [ ] **Step 1: Write the failing desktop adapter tests**

Follow the existing Files/model-runtime event tests. Register an `events.emit` action, call `registerAppconfigEvents(c)`, send a typed `appconfig.Event`, and assert exactly one `guievents.TaskEmit` with the fixed name and identical data. Serialise the data and assert forbidden names such as `path`, `token`, `command`, and `environment` are absent. Add `registerAppconfigEvents(nil)` coverage.

- [ ] **Step 2: Run the focused desktop test and confirm failure**

```bash
go test ./go/pkg/desktop -run 'TestRegisterAppconfigEvents' -count=1
```

Expected: compilation fails because the adapter does not exist.

- [ ] **Step 3: Implement and register the adapter**

Create:

```go
func registerAppconfigEvents(c *core.Core) {
    if c == nil { return }
    appconfig.Subscribe(c, func(c *core.Core, event appconfig.Event) {
        _ = emitCoreEvent(c, "lthn:desktop-controls:changed", event)
    })
}
```

Call `registerAppconfigEvents(s.opts.Core)` beside `registerFilesEvents`, `registerServicesEvents`, and `registerModelRuntimeEvents` in `desktop.go`.

- [ ] **Step 4: Verify the adapter and affected Go scope**

```bash
gofmt -w go/pkg/desktop/appconfig_events.go go/pkg/desktop/appconfig_events_test.go \
  go/pkg/desktop/desktop.go
go test ./go/pkg/appconfig ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/appconfig ./go/pkg/desktop ./go/cmd/lthn
```

Expected: all pass; close the development app first if port 9099 is occupied.

- [ ] **Step 5: Commit desktop event forwarding**

```bash
git add go/pkg/desktop/appconfig_events.go go/pkg/desktop/appconfig_events_test.go \
  go/pkg/desktop/desktop.go
git commit -m "feat(desktop): forward control change events"
```

### Task 3: Extend the typed NgRx state for revisions and conflicts

**Files:**

- Modify: `frontend/src/app/store/desktop-controls.models.ts`
- Modify: `frontend/src/app/store/desktop-controls.actions.ts`
- Modify: `frontend/src/app/store/desktop-controls.reducer.ts`
- Modify: `frontend/src/app/store/desktop-controls.reducer.spec.ts`
- Modify: `frontend/src/app/desktop/desktop-data-state.ts`
- Modify: `frontend/src/app/desktop/desktop-data-state-badge.spec.ts`

**Interfaces:**

- Consumes: revision strings from appconfig or the offline store.
- Produces: `DesktopControlsChangeNotice`, revision/stale/pending selectors, and explicit Reload/Keep editing actions.

- [ ] **Step 1: Write reducer and badge tests first**

Update fixtures to include `revision: '0'`. Add tests proving:

```ts
const pending = desktopControlsReducer(
  dirty,
  desktopControlsActions.externalChangePending({
    notice: {
      revision: "2",
      keys: ["desktop.theme.interface"],
      at: "2026-07-31T12:00:00Z",
    },
  }),
);
expect(pending.draft).toEqual(dirty.draft);
expect(pending.pendingExternalChange?.revision).toBe("2");

const reload = desktopControlsReducer(
  pending,
  desktopControlsActions.reloadExternalChange(),
);
expect(reload.draft).toEqual({});
expect(reload.pendingExternalChange).toBeNull();
```

Also prove load failure with existing controls sets `stale: true`, successful snapshots clear stale/pending state, and the data-state badge renders `Live data stale` with warning styling.

- [ ] **Step 2: Run the focused specs and confirm failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/store/desktop-controls.reducer.spec.ts \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts
```

Expected: compilation/assertion failures for the new contract.

- [ ] **Step 3: Implement the state contract**

Add:

```ts
export interface DesktopControlSnapshot {
  readonly revision: string;
  readonly controls: readonly DesktopControl[];
}

export interface DesktopControlsChangeNotice {
  readonly revision: string | null;
  readonly keys: readonly string[];
  readonly at: string | null;
}
```

Add actions `External change pending`, `Dismiss external change`, and `Reload external change`. Extend state with `revision`, `stale`, and `pendingExternalChange`. `loadSuccess` and `applyDraftSuccess` install the snapshot revision and clear pending/stale state; `loadFailure` keeps controls/draft and marks stale when prior controls exist. Reload clears the draft before the effect requests an authoritative load. Export selectors for revision, stale, and pending notice.

Add `'stale'` to `DesktopDataState`, label it `Live data stale`, and return the warning variant.

- [ ] **Step 4: Run and format the focused frontend tests**

```bash
npx prettier --write \
  src/app/store/desktop-controls.models.ts \
  src/app/store/desktop-controls.actions.ts \
  src/app/store/desktop-controls.reducer.ts \
  src/app/store/desktop-controls.reducer.spec.ts \
  src/app/desktop/desktop-data-state.ts \
  src/app/desktop/desktop-data-state-badge.spec.ts
npx ng test --watch=false \
  --include=src/app/store/desktop-controls.reducer.spec.ts \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts
```

Expected: all pass.

- [ ] **Step 5: Commit the renderer state contract**

```bash
git add frontend/src/app/store/desktop-controls.* \
  frontend/src/app/desktop/desktop-data-state.ts \
  frontend/src/app/desktop/desktop-data-state-badge.spec.ts
git commit -m "feat(frontend): track desktop control revisions"
```

### Task 4: Add connected and offline change providers

**Files:**

- Create: `frontend/src/app/desktop/desktop-controls-codec.ts`
- Create: `frontend/src/app/desktop/desktop-controls-offline.store.ts`
- Create: `frontend/src/app/desktop/desktop-controls-offline.store.spec.ts`
- Modify: `frontend/src/app/desktop/desktop-controls-bridge.service.ts`
- Modify: `frontend/src/app/desktop/desktop-controls-bridge.service.spec.ts`

**Interfaces:**

- Consumes: `DESKTOP_STORAGE`, same-origin storage events, `Events.On`, and Task 3 models.
- Produces: `DesktopControlsOfflineStore.settings/setMany/changes` and `DesktopControlsBridgeService.changes(): Observable<DesktopControlsChangeNotice>`.

- [ ] **Step 1: Write failing offline-store and bridge tests**

Prove the offline store:

- writes `{version: 1, revision, values}` to `lthn.desktop-controls.v1`;
- hydrates valid values across a new service instance;
- rejects malformed version/revision/value shapes and unknown keys;
- seeds known values from `lthn.prefs` without deleting it;
- emits a parsed notice for a matching `storage` event; and
- falls back to defaults when storage is unavailable.

Extend bridge tests with an injected event source and assert:

```ts
const notice = firstValueFrom(service.changes());
eventHandler?.({
  revision: "2",
  keys: ["desktop.theme.interface"],
  at: "2026-07-31T12:00:00Z",
});
await expect(notice).resolves.toEqual({
  revision: "2",
  keys: ["desktop.theme.interface"],
  at: "2026-07-31T12:00:00Z",
});
```

Malformed/extra event fields are ignored. Offline mode uses only the offline-store observable and never calls `Events.On` or the Wails bridge.

- [ ] **Step 2: Run the focused specs and confirm failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-controls-offline.store.spec.ts \
  --include=src/app/desktop/desktop-controls-bridge.service.spec.ts
```

Expected: compilation fails because the offline store and event observable do not exist.

- [ ] **Step 3: Extract the bounded codec**

Move snapshot parsing, change validation, value acceptance, deep copying, and the existing demo catalogue helpers from the bridge into `desktop-controls-codec.ts`. Require a non-empty revision no longer than 64 characters and exact bounded control values. Export only:

```ts
export function parseDesktopControlSnapshot(
  raw: unknown,
): DesktopControlSnapshot;
export function validateDesktopControlChanges(
  changes: readonly DesktopControlChange[],
): readonly DesktopControlChange[];
export function acceptsDesktopControlValue(
  control: DesktopControl,
  value: DesktopControlValue,
): boolean;
export function copyDesktopControlSnapshot(
  snapshot: DesktopControlSnapshot,
): DesktopControlSnapshot;
export function createDemoDesktopControlSnapshot(
  revision?: string,
): DesktopControlSnapshot;
```

- [ ] **Step 4: Implement the offline repository**

Use an injected storage-event observable so tests need no real browser tab. Keep the exact key and schema:

```ts
const STORAGE_KEY = "lthn.desktop-controls.v1";
interface StoredControls {
  readonly version: 1;
  readonly revision: string;
  readonly values: Readonly<Record<string, DesktopControlValue>>;
}
```

Generate a bounded opaque local revision for each successful draft, validate against the demo catalogue before writing, and expose storage notifications through RxJS. The legacy `lthn.prefs` mapping covers taskbar edge, icons, widgets, theme, brand, design, custom hue/name, wallpaper, reduced motion, and language. Never remove the legacy key.

- [ ] **Step 5: Implement bridge provider selection and connected event validation**

Add `DESKTOP_CONTROLS_EVENT_SOURCE`, wrapping only the fixed Wails event. `settings` and `setMany` delegate to the offline store when `connection.offline()` and otherwise use Wails calls plus the codec. `changes()` returns `offlineStore.changes()` offline, otherwise a cold Observable whose teardown calls the Wails unsubscribe function.

- [ ] **Step 6: Run focused tests, formatting, and build**

```bash
npx prettier --write src/app/desktop/desktop-controls-{codec,offline.store,bridge.service}*.ts
npx ng test --watch=false \
  --include=src/app/desktop/desktop-controls-offline.store.spec.ts \
  --include=src/app/desktop/desktop-controls-bridge.service.spec.ts
npm run build
```

Expected: all pass and the production build completes.

- [ ] **Step 7: Commit the provider layer**

```bash
git add frontend/src/app/desktop/desktop-controls-codec.ts \
  frontend/src/app/desktop/desktop-controls-offline.store.ts \
  frontend/src/app/desktop/desktop-controls-offline.store.spec.ts \
  frontend/src/app/desktop/desktop-controls-bridge.service.ts \
  frontend/src/app/desktop/desktop-controls-bridge.service.spec.ts
git commit -m "feat(frontend): add reactive controls providers"
```

### Task 5: Reconcile events in effects and expose conflicts in the panel

**Files:**

- Modify: `frontend/src/app/store/desktop-controls.effects.ts`
- Modify: `frontend/src/app/store/desktop-controls.effects.spec.ts`
- Modify: `frontend/src/app/desktop/preferences.service.ts`
- Modify: `frontend/src/app/desktop/preferences.service.spec.ts`
- Modify: `frontend/src/app/desktop/desktop-controls-panel.view.ts`
- Modify: `frontend/src/app/desktop/desktop-controls-panel.view.spec.ts`
- Modify: affected Settings/Control fixtures which construct `DesktopControlSnapshot` or mock selectors.

**Interfaces:**

- Consumes: `bridge.changes()`, `ConnectionManagerService.state`, and Task 3 selectors/actions.
- Produces: automatic clean refresh, dirty-draft conflict notices, reconnect reconciliation, and one offline persistence owner.

- [ ] **Step 1: Write failing effects, preference, and panel tests**

Effects tests use `Subject<DesktopControlsChangeNotice>` and a mock store. Prove:

- a different revision with a clean, non-saving store emits `load`;
- the current revision is ignored;
- a different revision with a dirty store emits `externalChangePending`;
- saving filters the initiating event echo;
- reconnect loads a clean store but marks a dirty store pending; and
- `reloadExternalChange` maps to `load`.

Update preference tests to prove neither connected nor offline projection writes `lthn.prefs`. Add panel tests for the notice text and buttons dispatching `reloadExternalChange` and `dismissExternalChange`, and for stale badge rendering.

- [ ] **Step 2: Run the focused specs and confirm failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/store/desktop-controls.effects.spec.ts \
  --include=src/app/desktop/preferences.service.spec.ts \
  --include=src/app/desktop/desktop-controls-panel.view.spec.ts
```

Expected: failures for missing effects, selectors, and notice UI.

- [ ] **Step 3: Implement RxJS reconciliation effects**

Inject `Store` and `ConnectionManagerService`. Build the event effect from `bridge.changes()` with `distinctUntilChanged` by revision and `withLatestFrom` revision/dirty/saving selectors. Ignore the current revision and saving state; dispatch `load` for clean state or `externalChangePending` for dirty state.

Convert the connection state signal with `toObservable`, use `pairwise`, and handle a transition into `connected` the same way, with a reconnect notice whose revision and timestamp are `null`. Map `reloadExternalChange` to `load`. Existing `switchMap` load semantics remain the bounded coalescing mechanism.

- [ ] **Step 4: Remove duplicate preference persistence**

Remove `DESKTOP_STORAGE`, `restore`, and `persist` from `PreferencesService`. Keep its DOM/token effect and `applySnapshot` projection unchanged so both connected and offline committed NgRx snapshots update the shell.

- [ ] **Step 5: Render the conflict and stale states**

Select `pendingExternalChange` and `stale` in `DesktopControlsPanelView`. Render a `role="status"` notice saying `Settings changed elsewhere.` with `data-action="reload-external-settings"` and `data-action="keep-editing-settings"` buttons. Dispatch only the typed actions. Return `stale` from `dataState` when prior data exists and refresh failed.

- [ ] **Step 6: Repair typed fixtures and run focused tests**

Add explicit revisions and mock-selector values to every affected spec found by:

```bash
rg -n "DesktopControlSnapshot|selectPendingExternalChange|selectStale" frontend/src/app
```

Then run:

```bash
npx prettier --write src/app/store/desktop-controls.effects*.ts \
  src/app/desktop/preferences.service*.ts \
  src/app/desktop/desktop-controls-panel.view*.ts
npx ng test --watch=false \
  --include=src/app/store/desktop-controls.effects.spec.ts \
  --include=src/app/store/desktop-controls.reducer.spec.ts \
  --include=src/app/desktop/desktop-controls-offline.store.spec.ts \
  --include=src/app/desktop/desktop-controls-bridge.service.spec.ts \
  --include=src/app/desktop/preferences.service.spec.ts \
  --include=src/app/desktop/desktop-controls-panel.view.spec.ts \
  --include=src/app/desktop/apps/settings.app.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts
```

Expected: all pass.

- [ ] **Step 7: Commit reactive reconciliation and UX**

```bash
git add frontend/src/app
git commit -m "feat(frontend): reconcile pushed control changes"
```

### Task 6: Add convergence guardrails and run completion gates

**Files:**

- Create: `scripts/verify-desktop-controls-sync.test.mjs`
- Modify: `TODO.md`
- Modify: `AGENTS.md`

**Interfaces:**

- Consumes: the completed Go/Angular event and offline-provider files.
- Produces: an executable architecture regression check and current developer guidance.

- [ ] **Step 1: Write the failing convergence test**

Add a Node test which reads source files and asserts:

```js
assert.match(appconfigEvents, /type Event struct/);
assert.match(desktopEvents, /lthn:desktop-controls:changed/);
assert.match(bridge, /changes\(\).*Observable/s);
assert.match(offlineStore, /lthn\.desktop-controls\.v1/);
assert.doesNotMatch(preferences, /DESKTOP_STORAGE|localStorage/);
```

Also assert the bridge's offline branch returns before Wails event registration.

- [ ] **Step 2: Run the contract test and confirm failure**

```bash
node --test scripts/verify-desktop-controls-sync.test.mjs
```

Expected: failure until the exact completed architecture is present.

- [ ] **Step 3: Update project guidance**

Document the appconfig event, offline store, revision/conflict semantics, and NgRx effects ownership in `AGENTS.md`. Add a completed, narrow appconfig-push item under `TODO.md` Shared bridge behaviour while leaving the broader cross-application reconciliation item open.

- [ ] **Step 4: Run all proportional and repository frontend gates**

```bash
gofmt -l go/
git diff --check
node --test scripts/verify-desktop-controls-sync.test.mjs
go test ./go/pkg/appconfig ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/appconfig -count=1
go vet ./go/pkg/appconfig ./go/pkg/desktop ./go/cmd/lthn
task verify:frontend
```

Expected: all commands pass. Record the fresh Angular test count, statement coverage, and build result from `task verify:frontend`; do not reuse an older percentage.

- [ ] **Step 5: Review the final branch scope and commit guardrails**

```bash
git status --short --branch
git diff --stat main...HEAD
git diff --check main...HEAD
git add scripts/verify-desktop-controls-sync.test.mjs TODO.md AGENTS.md
git commit -m "docs(desktop): guard reactive controls sync"
git status --short --branch
```

Expected: the branch is clean, contains only this feature and its approved design/plan, and remains unpushed.
