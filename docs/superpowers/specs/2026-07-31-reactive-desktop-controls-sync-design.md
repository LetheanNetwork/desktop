<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Reactive Desktop Controls Synchronisation Design

## Goal

Keep every Settings and Control surface attached to one reactive desktop-control
state, regardless of whether Angular is connected to the Lethean host or
running as an explicit offline browser demo.

Connected updates must travel through the existing Core action, Wails event,
and configurable WebSocket path. Offline updates must use a bounded browser
storage provider. NgRx/RxJS remains the synchronisation boundary in both modes;
Angular signals remain view adapters.

## Current state

The existing implementation already provides most of the required spine:

- `appconfig.Service.SetMany` validates a complete draft, commits once through
  CoreGO config's registered `io.Medium`, applies live controls only after the
  durable commit, and returns an authoritative snapshot;
- `DesktopControlsEffects` is the sole asynchronous Settings load/save path;
- `DesktopControlsPanelView` is shared by Settings and Control and renders NgRx
  selectors through Angular signals;
- `ConnectionManagerService` forwards server-pushed Wails event envelopes from
  the existing WebSocket into the normal `Events.On` dispatcher;
- `DESKTOP_STORAGE` provides localStorage with an in-memory fallback for
  restricted WebViews; and
- `PreferencesService` persists a separate offline `lthn.prefs` projection.

The missing link is change propagation. `appconfig` emits no Core event, so a
successful update cannot reconcile other WebViews. The in-memory demo snapshot
also does not survive a browser reload and duplicates values already persisted
by `PreferencesService`.

## Selected architecture

Add one typed appconfig change stream and one typed offline repository behind
the existing Angular bridge. Both feed the same RxJS effects and NgRx reducer.

```text
Connected
  appconfig SetMany
    -> durable io.Medium commit
    -> live control application
    -> typed Core event
    -> Wails event
    -> existing WebSocket fan-out
    -> RxJS effect
    -> NgRx store
    -> signal-based views

Offline demo
  versioned localStorage repository
    -> RxJS storage change stream
    -> NgRx store
    -> signal-based views
```

There is no second WebSocket, REST facade, state framework, or connected-mode
browser-storage fallback. IndexedDB remains suitable for future large offline
catalogues and retained demo history, but scalar desktop controls do not need
it.

## Go appconfig event contract

Add a typed `appconfig.Event` and `appconfig.Subscribe` using Core's ACTION bus,
matching the existing Files, managed-services, and model-runtime pattern.

The event contains only:

- an opaque, bounded revision token which changes for every successful
  committed snapshot;
- the bounded list of curated control keys changed by the committed draft; and
- an RFC3339-nanosecond timestamp.

It contains no values, credentials, provider paths, native paths, environment
data, or renderer-selected event names. The event is emitted exactly once after
a successful durable commit and live application. Empty drafts, rejected
drafts, failed writes, failed commits, and rollback paths emit nothing.

`Settings` and the successful `SetMany` result include the same revision. The
connected revision is deliberately transient: it deduplicates events within
one running host and is not product state. A backend restart replaces it, and
connection recovery performs an authoritative load when the draft is clean. A
dirty draft receives a pending-change notice and waits for explicit Reload or
Apply rather than ordering revisions across host lifetimes. Offline snapshots
persist their opaque local revision only so same-origin browser contexts can
deduplicate identical state.

The desktop host registers one adapter which publishes the Core event as:

```text
lthn:desktop-controls:changed
```

Its JSON shape is fixed to `revision`, `keys`, and `at`; unknown or missing
fields make the renderer reject the event.

The existing Wails transport performs event fan-out to connected WebViews and
separately served Angular clients.

## Angular connected provider

Extend `DesktopControlsBridgeService` with a bounded RxJS change observable.
It wraps `Events.On('lthn:desktop-controls:changed', ...)`, validates the exact
event envelope, and unregisters the listener when the subscription ends. It
does not expose arbitrary event names or raw Wails payloads.

`DesktopControlsEffects` owns the subscription. On a valid event:

1. an event whose revision matches the current snapshot is ignored, which
   removes the initiating window's save echo;
2. a different event refreshes `Settings` automatically when there is no unsaved
   draft;
3. a different event received while a draft is dirty records a pending external
   change without replacing the draft; and
4. a transition from disconnected or reconnecting to connected reloads a clean
   store, while a dirty store records a pending external change and waits for
   explicit Reload or Apply.

The successful `SetMany` response remains authoritative for the initiating
surface. Event bursts are coalesced before refresh so one committed batch does
not cause duplicate reads in a WebView.

## Offline browser provider

Add `DesktopControlsOfflineStore`, a narrow offline repository backed by
`DESKTOP_STORAGE`. It owns `lthn.desktop-controls.v1`, a bounded envelope with
the exact shape `{ version: 1, revision, values }`. `values` contains at most
the curated control count and only validated boolean, finite number, or bounded
string values. Unknown keys and invalid values are discarded during parsing.

The repository provides the same operations needed by the effects:

- load the current validated snapshot;
- apply one validated draft atomically;
- expose same-origin `storage` events as an RxJS change stream for other
  browser tabs; and
- fall back to isolated in-memory storage when localStorage is unavailable.

The current `lthn.prefs` document may be read once as a compatibility seed.
The migration writes the canonical versioned desktop-controls envelope but
does not delete the legacy key. Afterwards, `PreferencesService` projects
committed NgRx snapshots into DOM-facing signals and no longer writes a second
competing copy of the same controls.

Explicit offline mode registers no Wails listener, opens no WebSocket, and
makes no Wails binding call. Browser storage is never consulted as connected
product authority.

## Store and user experience

Extend the desktop-controls state with:

- the current connected or offline revision; and
- an optional pending external-change notice containing revision, changed
  keys, and timestamp.

If an external update arrives while the draft is clean, the interface updates
automatically. If the draft is dirty, the panel keeps the user's edits and
shows a calm "Settings changed elsewhere" notice with two existing-style
actions:

- **Reload**, which explicitly discards the draft and loads the latest
  snapshot; and
- **Keep editing**, which dismisses the notice while retaining the draft.

Applying a retained draft sends only its validated changed keys. The returned
complete snapshot then includes every current backend value, so unrelated
external changes are preserved. If both parties changed the same key, the
user's later explicit Apply is the winning commit.

Load or refresh failure keeps the last valid snapshot and any draft visible,
marks the data stale, and offers Retry. A malformed event is ignored and does
not mutate state. A malformed offline document falls back to safe demo
defaults without overwriting the invalid document until the user makes a valid
change.

## Component boundaries

The implementation remains narrow:

- `go/pkg/appconfig/` owns revision generation, event publication, and the
  authoritative Settings snapshot;
- `go/pkg/desktop/` translates the typed Core event to one fixed Wails event;
- `DesktopControlsBridgeService` validates the connected event and selects the
  connected or offline provider;
- a new offline repository owns versioned browser persistence;
- `DesktopControlsEffects` owns RxJS subscription, reconciliation, refresh,
  and retry orchestration;
- the desktop-controls reducer owns revision and conflict state; and
- `DesktopControlsPanelView` only renders selectors and dispatches typed user
  intents.

No component calls Wails, localStorage, or IndexedDB directly.

## Verification

Go Good/Bad/Ugly tests prove:

- a successful batch emits one typed event after commit with known keys and
  the same revision returned by `Settings`;
- empty, duplicate, unknown, invalid, write-failed, and commit-failed batches
  emit no event;
- rollback still restores prior values and applies no live action;
- subscription safely ignores nil Core and nil listeners; and
- the desktop adapter emits only `lthn:desktop-controls:changed`.

Angular tests prove:

- connected mode registers and cleans up exactly one Wails event listener;
- offline mode registers no Wails listener and makes no binding call;
- the initiating revision is deduplicated;
- clean stores refresh from a newer event;
- dirty drafts survive newer events and expose Reload/Keep editing;
- reconnect refreshes a clean store and preserves a dirty draft behind the
  pending-change notice;
- malformed events cannot mutate the store;
- offline values survive reload through the versioned storage adapter;
- malformed, oversized, and unknown offline values fail safely;
- same-origin storage notifications reconcile a second browser context; and
- `PreferencesService` projects snapshots without maintaining duplicate
  persistence.

Focused package, Angular, event-convergence, production-build, and full
`task verify:frontend` checks remain the completion gates.

## Out of scope

This slice does not:

- introduce IndexedDB for scalar settings;
- persist connected settings in browser storage;
- watch config files changed by a separate operating-system process;
- create a generic event/resource framework for every desktop application;
- standardise every application's live/demo/loading/stale presentation; or
- add polling where the existing event and reconnect paths are sufficient.

Those broader concerns can follow after this vertical slice proves the shared
NgRx/RxJS reconciliation contract.
