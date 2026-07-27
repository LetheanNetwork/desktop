<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Desktop State, Terminal Resilience, and Native Integration Design

## Goal

Turn four existing partial implementations into one dependable desktop
experience:

1. Settings becomes a validated, recoverable control centre rather than a
   mixture of CoreGO config and renderer-local preferences.
2. Terminal tabs survive view remounts and application restarts, while live
   PTY sessions recover cleanly from Wails transport interruptions.
3. the browser desktop restores its windows and workspace from a versioned
   application document instead of treating `localStorage` as product state;
   and
4. native file opening, file drops, deep links, notifications, permissions,
   and tray actions enter Angular through one typed capability boundary.

The work must preserve the existing applications, desktop modes, window UX,
terminal features, tray lifetime, and deterministic browser demo. It extends
the current Angular 22 and CoreGO architecture; it does not introduce another
frontend, state framework, native supervisor, or automatic background
service.

## Current state

The repository already contains substantial foundations:

- `SettingsApp` renders a typed `appconfig` catalogue and NgRx effects persist
  each control through CoreGO config. Theme and reduced-motion values are
  projected into `PreferencesService`.
- `PreferencesService` separately stores taskbar position, wallpaper, brand,
  design, language, icons, widgets, and related UI values in
  `localStorage`.
- `DesktopEffects` serialises the complete inner desktop window state to
  `localStorage`, hydrates it during startup, and lets the reducer discard
  unknown applications. CoreGUI separately owns the outer native window
  state.
- `go/pkg/terminal` owns a process-backed PTY pool and a bounded scrollback
  ring. `Attach` is idempotent and replays the ring, but the Angular terminal
  neither reacts to connection recovery nor persists its tab workspace.
- macOS and the Windows NSIS installer already declare `.lthn` and `lthn://`
  associations. Linux declares the URL scheme. Deep links, tray navigation,
  native lifecycle events, notification actions, and file drops are
  re-broadcast onto `lthn:*` events.

The important defect in the native path is that opened-file and dropped-file
events currently include absolute host paths. Those paths cross into the
renderer before `go-io` or the Files capability model has authorised them.
That must be repaired before building richer Open With behaviour.

## Compatibility and security contract

The implementation must preserve these decisions:

- all file-backed product state ultimately flows through a registered
  `dappco.re/go/io.Medium`; there is no raw `os`, `path/filepath`, `syscall`,
  or browser-storage fallback in connected product mode;
- an unavailable Medium fails closed for persistence while the visible
  desktop remains operable from safe defaults;
- provider roots, absolute native paths, credentials, environment values,
  terminal contents, typed commands, and arbitrary process arguments never
  cross the renderer contract or enter desktop-state documents;
- explicit offline transport uses isolated in-memory state, makes no Wails
  call, and installs no Wails event listener;
- `appconfig` remains the owner of scalar application and native policy;
- the new structured state service does not duplicate CoreGUI's outer native
  window state;
- terminal process identity and output remain transient runtime state;
- registration, hydration, Settings reads, and state restoration never start
  a process or request an operating-system permission;
- permission requests and native notifications are explicit user or trusted
  package actions;
- existing apps, routes, window interaction, grouping, shell layouts,
  Terminal search, agent tabs, tray behaviour, and design adaptations remain
  available;
- British English, EUPL-1.2, and the no-paywall policy remain in force.

## Approaches considered

### Focused feature services over one safe state spine — selected

Keep scalar Settings in `appconfig`, add a narrow `desktopstate.Service` for
structured shell and Terminal workspace documents, keep PTYs in
`go/pkg/terminal`, and translate operating-system events in trusted Go before
they are published as typed host intents.

This gives each concern one owner while sharing the same Medium-backed
commit/recovery rules.

### One general desktop service

A single service could own Settings, window state, terminals, permissions,
files, notifications, and tray actions. It would minimise binding count, but
would become a second composition root and blur durable state, transient
processes, application policy, and native authority.

### Continue feature-local browser persistence

Keeping `PreferencesService`, `DesktopEffects`, and Terminal tabs independent
would be the smallest patch. It would leave product state tied to one WebView
profile, make remote GUI transport inconsistent, and bypass the required
Medium security boundary.

## Ownership and data boundaries

### `appconfig`

`go/pkg/appconfig` remains the curated Settings authority. It gains the
remaining renderer preferences under stable `desktop.*` keys and a bounded
batch commit operation. Angular never receives a general CoreGO config
editor.

Core composition must supply the CoreGO config service with a trusted,
registered Medium rooted at the existing Lethean configuration location.
The canonical `~/Lethean/conf/lthn.yaml` location remains compatible, but
reads, writes, staging, recovery, and metadata operations flow through that
Medium. The renderer receives neither the root nor the absolute config path.

### `desktopstate`

Add `go/pkg/desktopstate` with the canonical CoreGO service shape:

```go
type Options struct {
    Medium io.Medium
}

type Service struct {
    // private Medium, document store, revision state, and locks
}

func NewService(options Options) *Service
func (s *Service) Register(c *core.Core) core.Result
func Register(c *core.Core) core.Result
```

The service exposes typed methods for shell sessions and Terminal workspaces;
it is not a renderer-addressable arbitrary JSON or path store. Its free
`Register` function resolves the named application I/O service and fails
closed when no Medium is registered.

Documents live below provider-relative application paths:

```text
desktop/state/shell-session.json
desktop/state/terminal-workspace.json
desktop/state/.staging/
```

Every document has a supported version, monotonic revision, payload, and
update time. Saves accept the caller's expected revision so two renderer
windows cannot silently overwrite newer state.

Writes use the Files runtime pattern: validate and bound the complete model,
write a mode-`0600` staging document, validate read-back, rename the previous
document to a recoverable backup, rename staging into place, and remove the
backup only after success. A failed final rename attempts recovery. Startup
uses only a complete supported document or a valid backup; malformed,
oversized, unsupported, or conflicting state is reported rather than
overwritten.

### Terminal runtime

`go/pkg/terminal` continues to own PTY creation, output, resize, close, and
the in-memory session pool. `desktopstate` stores only the user's tab
workspace intent. It does not persist PTYs, process IDs, scrollback, shell
input, commands, or environment.

### Host intents

Raw lifecycle values are trusted-host inputs, not renderer events. A narrow
Go adapter resolves them against `go/pkg/office/files`, deep-link allowlists,
notification catalogues, or tray targets and then emits one discriminated
host-intent envelope. Angular consumes that envelope through a single
`DesktopHostIntentService`.

## Settings design

The existing control catalogue gains the current `PreferencesService` values:

- taskbar edge;
- interface mode, brand, design, custom accent name and hue;
- wallpaper;
- language;
- desktop icons and widgets; and
- reduced motion.

Existing keys such as interface theme and reduced motion remain canonical;
the change must not introduce aliases for values already in the catalogue.

`SettingsApp` edits an immutable draft rather than dispatching a durable write
on every input event. It provides:

- Apply for the validated changed set;
- Discard to restore the latest committed snapshot;
- Reset to write catalogue defaults for the selected controls;
- per-control validation without erasing the previous committed value;
- one visible saving/error status; and
- a consolidated restart-required summary after a successful commit.

`appconfig.Service.SetMany` accepts only known catalogue keys and bounded
values. It validates the complete request before mutation, rejects duplicate
keys, snapshots the current config document through its Medium, commits the
batch, and applies live CoreGUI actions only after durable success. A failed
commit restores the prior document and reloads the previous effective values
through CoreGO config. The returned snapshot is authoritative, so NgRx either
commits the new draft or rolls the UI back without guessing.

`PreferencesService` becomes a renderer projection of the loaded catalogue.
It applies signals and DOM attributes but does not independently persist in
connected mode. Offline demo mode composes a deterministic in-memory Settings
provider with the same catalogue and draft behaviour.

## Shell session restoration

The shell-session payload contains only bounded, catalogue-derived UI state:

```text
version/revision
view and device-preview mode
focused window id and next z value
windows:
  id, registered app id, registered sub-route/system tab
  x, y, width, height, z
  minimised/maximised state and group id
```

The backend rejects unknown fields that could carry execution or provider
authority and enforces maximum window counts, identifier lengths, dimensions,
and document size. The Angular reducer remains responsible for dropping app
IDs no longer present in `APPS`, normalising routes, and producing the safe
default desktop.

An NgRx effect loads the Medium document before ordinary persistence starts.
It reconciles restored geometry with the current viewport and visible work
area, ensuring every restored window has a reachable title bar. Group
membership, focus, z-order, view mode, and device preview are otherwise
preserved.

Window movement and resize update NgRx immediately. Durable saves are
debounced so pointer movement does not produce a file write per event. Close,
launch, minimise, maximise, grouping, route, view, and device changes request
the same bounded save path. Revision conflicts trigger a reload and
deterministic reconciliation rather than last-writer-wins data loss.

The first successful connected load may import the current `lthn.desktop`
browser value after parsing it through the same reducer and backend
validation. Once the Medium document is committed, the migration marker is
part of that document and browser storage is removed. `localStorage` remains
available only to the explicit offline demo provider.

CoreGUI's native outer-window state remains unchanged and separate.

## Terminal resilience and workspace restoration

### Cursor-aware output

The PTY ring gains a monotonic byte cursor. Output chunks carry:

```ts
interface TerminalChunk {
  readonly start: number;
  readonly end: number;
  readonly data: string;  // base64
  readonly reset: boolean;
}
```

`Attach` accepts an optional `after` cursor:

- when the cursor is still represented by the ring, only missing bytes are
  replayed;
- when the cursor is older than the retained ring, one `reset` snapshot
  replaces the renderer's stale scrollback; and
- live chunks continue from the returned cursor without a gap between replay
  and subscription.

Angular tracks the last accepted cursor, drops duplicate data, detects gaps,
and requests another attach. A reset clears xterm before applying the bounded
snapshot. This prevents the current full-ring duplication that a naïve
reconnect would cause.

`AgentTerminalSession` observes `ConnectionManagerService.state`. On a
transition back to `connected`, it keeps the existing Wails event handlers
and reattaches the in-memory session from its last cursor. If the pool no
longer knows the session, the tab becomes exited and offers a fresh shell
rather than silently running a replacement command.

### Persisted workspace

The Terminal workspace document stores:

- stable tab keys, order, reader-facing title, and active key;
- a trusted repository or Files mount identifier plus provider-relative
  working path;
- whether the intent is an ordinary shell or a discoverable shared-agent tab;
  and
- presentation preferences that are not terminal content.

It never stores an absolute working directory, command array, environment,
shell input, output, transient session ID, PID, access token, or credential.
Generic renderer-supplied command execution is replaced by trusted
Go-registered command intents. Existing trusted packages may continue to use
`terminal.Spawn` for agent sessions.

Within one Core lifetime a reconnect reuses the transient session ID. After
an application restart, an ordinary saved shell intent opens a fresh shell in
its authorised workspace. Shared-agent tabs are restored only when `List`
reports the trusted session as live; otherwise they remain visibly exited.

Adding, closing, reordering, renaming, or activating a tab schedules a
debounced workspace save. Offline demo mode uses an isolated in-memory tab
store and never creates a PTY or Wails subscription.

## Native integration

### Files and Open With

`ActionOpenedWithFile` and `ActionFilesDropped` no longer emit native paths.
Trusted Go resolves each item as follows:

1. reuse an existing Files mount when the item is within an authorised local
   Medium;
2. otherwise create a session-scoped, least-authority Medium for the selected
   item and register an opaque read-only Files mount;
3. produce only mount ID, provider-relative path, safe display name, media
   type, and bounded drop coordinates; and
4. route `.lthn` configuration items to the Settings import intent and other
   supported items to Files.

Ephemeral mounts are not persisted and disappear with the host Core. A
provider that cannot safely produce a capability returns an unavailable
intent; it never falls back to sending the path.

The existing macOS and NSIS declarations are retained. Linux gains the
`application/x-lethean` MIME declaration and a file-capable `%U` invocation.
The Windows MSIX manifest receives the real `lthn` executable/product
metadata plus protocol and `.lthn` declarations. Contract tests keep the
three desktop packaging paths aligned. Mobile association changes remain a
separate host-specific tranche unless the active Wails mobile runtime exposes
the same capability contract during implementation.

### Deep links and tray

The current allowlisted `lthn://` parser and catalogue-derived Angular routing
remain authoritative. `DesktopHostIntentService` absorbs the existing
`navigate` and `lthn:tray:open` behaviours into one typed dispatch path while
retaining compatibility events during migration. Unknown app IDs, resources,
plugin codes, and tray targets remain inert.

### Notifications

Native notifications use bounded server-owned IDs and action IDs. Angular may
request a known notification kind with safe display parameters; it cannot
attach an arbitrary command, URL, path, or executable action. Click, action,
and dismiss events return through the host-intent service and route using the
same application catalogue.

The Angular notification stack remains the in-desktop presentation. Native
notifications complement it for background/tray operation rather than
replacing it.

### Permissions

Application policy from `go/pkg/permissions` and actual operating-system
permission status are shown separately. The host reports only states it can
verify: granted, denied, prompt, restricted, unsupported, or unknown.
Unsupported platforms do not pretend a permission is granted.

Requests are available only from an explicit Settings action and only for a
known capability. Reading Settings, restoring a session, or launching the
desktop never prompts.

## Angular structure

The intended additions are:

```text
frontend-ng/src/app/desktop/
  desktop-state-bridge.service.ts
  desktop-host-intent.service.ts
  terminal-workspace.models.ts

frontend-ng/src/app/store/
  desktop-session.actions.ts
  desktop-session.effects.ts
```

Existing `DesktopControlsEffects`, `PreferencesService`,
`WindowManagerService`, `DesktopEffects`, `DeepLinkNavigationService`,
`AgentsTerminalSurface`, and `AgentTerminalSession` are adapted in place.
There is no parallel window manager, route registry, preference service, or
Terminal component.

Bridges validate unknown Wails payloads before exposing typed values. Offline
providers are selected from the existing explicit transport mode, not from
catching a connected failure.

## Error behaviour

- Settings validation keeps the committed snapshot visible and identifies
  invalid controls.
- A config persistence or recovery failure reports Settings unavailable and
  never applies the uncommitted live change.
- Missing or corrupt desktop state starts the safe default desktop and shows
  persistence as unavailable; the evidence is not overwritten automatically.
- Terminal disconnection retains the visible xterm buffer, disables writes,
  and shows reconnect status. Recovery uses the cursor protocol.
- A lost Terminal session is exited, not silently recreated with execution
  data.
- An unauthorised host file produces a bounded unavailable intent without
  revealing its path.
- Unknown native events, notification actions, routes, and permission values
  are ignored or represented as unsupported; they do not widen authority.

## Testing

Development follows red-green TDD.

Go tests use focused Good/Bad/Ugly cases and runnable examples for:

- Medium-required registration and no raw fallback;
- version validation, size/count bounds, revision conflicts, staged commits,
  failed rename recovery, valid backup recovery, and corrupt evidence;
- Settings batch validation, rollback, live-apply ordering, defaults, and
  config Medium use;
- Terminal cursor replay, duplicate suppression inputs, ring overrun reset,
  attach races, unknown sessions, and unchanged PTY lifecycle;
- host-path containment, ephemeral capability creation, payload redaction,
  allowlisted routing, notification actions, and permission states; and
- explicit no-auto-start/no-permission-prompt startup behaviour.

Angular tests cover:

- Settings draft, Apply, Discard, Reset, restart summary, rollback, and demo
  isolation;
- shell hydration, migration, debounced save, viewport reconciliation,
  revision conflict, and unavailable persistence;
- Terminal workspace hydration, reconnect/reattach, cursor deduplication,
  reset, missing session, teardown, and offline behaviour;
- host-intent payload rejection and routing for files, URLs, notifications,
  tray targets, and permissions; and
- preservation of current route, window, grouping, Terminal, and shell
  presenter behaviour.

Packaging contract tests assert matching application identity, `.lthn` file
type, `lthn://` scheme, and executable invocation for macOS, Linux, Windows
NSIS, and Windows MSIX.

Security assertions serialise renderer payloads and reject absolute POSIX
roots, Windows drive/UNC paths, environment values, command arrays, and
provider credentials.

After focused checks, the tranche runs the repository Go tests and vet,
Angular tests, convergence contracts, production frontend build,
`git diff --check`, and the existing no-regression audit. The external audit's
pre-existing backlog is reported honestly rather than described as an
all-zero gate.

## Delivery sequence

1. Add the Medium-backed config and `desktopstate` document foundations.
2. Repair opened-file/drop path emission with opaque host intents.
3. Implement Settings draft and batch commit.
4. Move inner desktop session hydration and persistence to
   `desktopstate`.
5. Add Terminal cursor replay, reconnect handling, and workspace persistence.
6. complete native notification, permission, tray, and cross-platform
   association contracts.
7. remove connected-mode browser persistence only after migration tests pass.
8. run full verification, inspect exact staged scope, and push `main`.

Each step is independently testable and committed. Existing user changes in
`go.work.sum` and `.playwright-mcp/` remain outside every commit.

## Non-goals

- persisting terminal contents, commands, secrets, PIDs, or PTYs across a
  Core restart;
- restoring running background services or enabling login startup;
- replacing CoreGUI's outer native-window state manager;
- exposing arbitrary config keys, host paths, commands, notification URLs, or
  permission names to Angular;
- implementing a general remote filesystem-to-local-process bridge;
- redesigning or removing existing desktop applications and interaction
  features; and
- treating mobile association behaviour as complete without a verified Wails
  mobile capability path.
