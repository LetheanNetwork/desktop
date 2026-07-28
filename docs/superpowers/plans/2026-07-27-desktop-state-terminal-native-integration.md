<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Desktop State, Terminal Resilience, and Native Integration Implementation Plan

> **For Codex:** REQUIRED SKILLS: use `superpowers:test-driven-development`
> for every behaviour change, `angular-developer` for Angular work,
> `superpowers:executing-plans` to execute this plan task by task, and
> `superpowers:verification-before-completion` before claiming completion.

**Goal:** Make Settings transactional, persist the inner desktop and Terminal
workspace through `io.Medium`, recover live Terminal streams after transport
loss, and route native host events through opaque typed capabilities.

**Architecture:** Scalar policy stays in `appconfig`. New structured state is
owned by a narrow `desktopstate.Service` backed by the registered application
Medium. PTYs remain transient in `go/pkg/terminal`. Native paths are consumed
only by trusted Go and resolved into Files capabilities before Angular sees a
host intent. Angular adapts the existing NgRx, preference, window, Terminal,
deep-link, and shell services rather than creating parallel UI systems.

**Technology:** Go 1.26, CoreGO `core.Result` and service lifecycle,
`dappco.re/go/io.Medium`, CoreGO config, CoreGUI/Wails 3, Angular 22 standalone
components, signals, NgRx 21, Vitest/jsdom, xterm.js, npm.

---

## Working rules

- Work on the current explicitly authorised `main` branch.
- Stage named files only. Never stage the user's `go.work.sum` or
  `.playwright-mcp/`.
- Start each behaviour with a focused failing test and observe the expected
  failure before implementation.
- Keep commits scoped to the task listed below.
- Use British English in identifiers where natural, copy, docs, and tests.
- Never add raw filesystem access as a convenience path.
- Explicit offline mode makes no Wails call and installs no Wails event
  listener.

## Task 1: Build the Medium-backed desktop document store

**Files:**

- Create: `go/pkg/desktopstate/document.go`
- Create: `go/pkg/desktopstate/document_test.go`
- Create: `go/pkg/desktopstate/errors.go`

**Step 1: Write failing Good/Bad/Ugly tests**

Cover:

- a valid versioned document round-trips through
  `coreio.NewMemoryMedium()`;
- a nil Medium fails closed;
- malformed JSON and unsupported versions fail without overwriting evidence;
- a failed final rename restores the prior document;
- a valid backup is recovered when the primary is absent;
- an invalid backup is retained and reported;
- expected-revision conflicts do not write; and
- mode `0600`, maximum bytes, and parent/staging paths are enforced.

The store API is package-private and typed around a caller-supplied codec:

```go
type envelope[T any] struct {
    Version   int       `json:"version"`
    Revision  uint64    `json:"revision"`
    UpdatedAt time.Time `json:"updatedAt"`
    Payload   T         `json:"payload"`
}

type document[T any] struct {
    medium coreio.Medium
    path   string
    limits documentLimits
}

func (d *document[T]) Load() core.Result
func (d *document[T]) Save(expected uint64, payload T) core.Result
```

**Step 2: Run the red test**

```bash
go test ./go/pkg/desktopstate -run 'TestDocument' -count=1
```

Expected: compile failure because the document store does not exist.

**Step 3: Implement the smallest passing store**

Use only Medium methods for `Read`, `WriteMode`, `EnsureDir`, `Stat`,
`Rename`, `List`, and `Delete`. Follow the audited staging/backup sequence in
`go/pkg/office/files/runtime.go` without importing Files internals.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/desktopstate -run 'TestDocument' -count=1
go test -race ./go/pkg/desktopstate -run 'TestDocument' -count=1
git diff --check -- go/pkg/desktopstate
git add go/pkg/desktopstate/document.go \
  go/pkg/desktopstate/document_test.go \
  go/pkg/desktopstate/errors.go
git commit -m "feat: add medium-backed desktop documents"
```

## Task 2: Add typed shell-session and Terminal-workspace services

**Files:**

- Create: `go/pkg/desktopstate/models.go`
- Create: `go/pkg/desktopstate/models_test.go`
- Create: `go/pkg/desktopstate/service.go`
- Create: `go/pkg/desktopstate/service_test.go`
- Create: `go/pkg/desktopstate/wails.go`
- Create: `go/pkg/desktopstate/desktopstate_example_test.go`
- Modify: `go/cmd/lthn/app.go`
- Modify: `go/pkg/desktop/desktop.go`
- Create: `go/pkg/desktop/desktopstate_binding_test.go`

**Step 1: Write failing model and service tests**

Define bounded renderer models:

```go
type Window struct {
    ID, App, Sub, SystemTab, Group string
    X, Y, Width, Height, Z         int
    Min, Max                       bool
}

type ShellSession struct {
    View, Device, FocusID string
    Z                     int
    Windows               []Window
    MigratedBrowserState  bool
}

type WorkspaceRef struct {
    MountID, Path, Repository string
}

type TerminalTab struct {
    Key, Title, Kind string
    Workspace        WorkspaceRef
    SharedAgentID    string
}

type TerminalWorkspace struct {
    ActiveKey string
    Tabs      []TerminalTab
}
```

Tests must reject unknown view/device/kind values, duplicate IDs/keys,
unregistered-looking identifiers, absolute/traversing paths, excessive
windows/tabs, oversized strings, command/environment-shaped fields, and stale
revisions.

Test `Register` resolves the named application `*io.Service`, uses its Medium,
and fails closed without it. Test the typed Wails wrapper delegates only
`LoadShellSession`, `SaveShellSession`, `LoadTerminalWorkspace`, and
`SaveTerminalWorkspace`.

**Step 2: Run the red tests**

```bash
go test ./go/pkg/desktopstate ./go/pkg/desktop \
  -run 'DesktopState|ShellSession|TerminalWorkspace' -count=1
```

**Step 3: Implement and compose**

Register `desktopstate` after the application `io` service in
`newAppCore`. Resolve that same instance in `desktop.Run` and bind its Wails
wrapper. Do not create a sibling service in the GUI composition.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/desktopstate ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/desktopstate ./go/pkg/desktop ./go/cmd/lthn
git diff --check
git add go/pkg/desktopstate go/cmd/lthn/app.go \
  go/pkg/desktop/desktop.go go/pkg/desktop/desktopstate_binding_test.go
git commit -m "feat: add typed desktop state service"
```

## Task 3: Put CoreGO config and Settings batches on the Medium boundary

**Files:**

- Modify: `go/cmd/lthn/app.go`
- Modify: `go/cmd/lthn/app_test.go`
- Modify: `go/pkg/appconfig/service.go`
- Modify: `go/pkg/appconfig/service_test.go`
- Modify: `go/pkg/appconfig/service_example_test.go`
- Modify: `go/pkg/appconfig/resolver.go` if catalogue resolution needs shared
  helpers

**Step 1: Write failing tests**

Add tests proving:

- the application CoreGO config service is constructed with a registered
  config Medium while preserving the existing config location;
- `SetMany` validates all keys before changing any value;
- duplicate keys and invalid values reject the entire batch;
- one durable commit produces the complete effective snapshot;
- live CoreGUI actions occur only after successful commit;
- a failed commit restores the previous document and in-memory effective
  values; and
- Settings no longer returns an absolute `config_path`.

Use a fault-injecting Medium rather than real user directories.

Input:

```go
type Change struct {
    Key   string `json:"key"`
    Value any    `json:"value"`
}

func (s *Service) SetMany(changes []Change) core.Result
```

Keep `Set` as a one-change compatibility delegate.

**Step 2: Run the red tests**

```bash
go test ./go/pkg/appconfig ./go/cmd/lthn \
  -run 'SetMany|ConfigMedium|Settings' -count=1
```

**Step 3: Implement**

Create and register the trusted config Medium at composition time, pass it to
`config.ServiceOptions.Medium`, and use a provider-relative config filename.
Snapshot, restore, and reload config only through config/Medium APIs.

Add the remaining preference keys to the curated definitions, reusing
existing theme/reduced-motion keys.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/appconfig ./go/cmd/lthn -count=1
go vet ./go/pkg/appconfig ./go/cmd/lthn
git diff --check
git add go/cmd/lthn/app.go go/cmd/lthn/app_test.go \
  go/pkg/appconfig/service.go go/pkg/appconfig/service_test.go \
  go/pkg/appconfig/service_example_test.go go/pkg/appconfig/resolver.go
git commit -m "feat: commit desktop settings atomically"
```

Only add `resolver.go` if it actually changed.

## Task 4: Repair native file events before expanding host UX

**Files:**

- Create: `go/pkg/office/files/host_items.go`
- Create: `go/pkg/office/files/host_items_test.go`
- Create: `go/pkg/desktop/host_intents.go`
- Create: `go/pkg/desktop/host_intents_test.go`
- Modify: `go/pkg/desktop/sysevents.go`
- Modify: `go/pkg/desktop/sysevents_test.go` if present; otherwise extend the
  new host-intent tests

**Step 1: Write the red security tests**

Prove:

- a host item inside an existing local mount resolves to mount ID and relative
  path;
- an outside selected item receives an opaque session-scoped read-only mount;
- traversal, non-file providers, duplicate/oversized drops, and unavailable
  Medium fail closed;
- emitted JSON contains no absolute POSIX, drive-letter, or UNC path;
- target payloads retain only bounded target ID and coordinates; and
- open-file/drop lifecycle actions emit `lthn:host:intent`, never raw
  `lthn:app:opened-file` or raw `files` arrays.

Host-item resolution is an internal trusted Go method, not a Wails method.

**Step 2: Run the red tests**

```bash
go test ./go/pkg/office/files ./go/pkg/desktop \
  -run 'HostItem|HostIntent|OpenedWithFile|FilesDropped' -count=1
```

**Step 3: Implement**

Add a discriminated envelope:

```go
type HostIntent struct {
    Kind         string         `json:"kind"`
    Items        []HostItemView `json:"items,omitempty"`
    Navigation   map[string]string `json:"navigation,omitempty"`
    Notification *NotificationIntent `json:"notification,omitempty"`
    TrayTarget   string         `json:"trayTarget,omitempty"`
    Target       *DropTarget    `json:"target,omitempty"`
}
```

Keep raw paths entirely inside the lifecycle action handler. Resolve them
through the Core-registered Files service before emission.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/office/files ./go/pkg/desktop -count=1
go vet ./go/pkg/office/files ./go/pkg/desktop
git diff --check
git add go/pkg/office/files/host_items.go \
  go/pkg/office/files/host_items_test.go \
  go/pkg/desktop/host_intents.go \
  go/pkg/desktop/host_intents_test.go \
  go/pkg/desktop/sysevents.go
git commit -m "fix: hide native paths behind files capabilities"
```

## Task 5: Make Angular Settings a draft/commit workflow

**Files:**

- Modify: `frontend/src/app/desktop/desktop-controls-bridge.service.ts`
- Modify:
  `frontend/src/app/desktop/desktop-controls-bridge.service.spec.ts`
- Modify: `frontend/src/app/store/desktop-controls.actions.ts`
- Modify: `frontend/src/app/store/desktop-controls.effects.ts`
- Modify: `frontend/src/app/store/desktop-controls.effects.spec.ts`
- Modify: `frontend/src/app/store/desktop-controls.reducer.ts`
- Modify: `frontend/src/app/store/desktop-controls.reducer.spec.ts`
- Modify: `frontend/src/app/store/desktop-controls.models.ts`
- Modify: `frontend/src/app/desktop/preferences.service.ts`
- Create: `frontend/src/app/desktop/preferences.service.spec.ts`
- Modify: `frontend/src/app/desktop/apps/settings.app.ts`
- Modify: `frontend/src/app/desktop/apps/settings.app.spec.ts`

**Step 1: Read the Angular references required by the local Angular skill**

Read the complete Angular references for signals, services/DI, testing, and
forms before editing.

**Step 2: Write failing tests**

Test:

- editing changes only the draft;
- Apply sends one bounded `SetMany` request;
- Discard restores committed values;
- Reset writes catalogue defaults into the draft;
- failure restores committed UI and preserves an accessible error;
- successful restart-required changes produce one summary;
- `PreferencesService` projects the authoritative snapshot and does not write
  browser storage in connected mode; and
- offline Settings uses in-memory state and never calls the bridge.

**Step 3: Run the red tests**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/settings.app.spec.ts \
  --include=src/app/desktop/preferences.service.spec.ts \
  --include=src/app/desktop/desktop-controls-bridge.service.spec.ts \
  --include=src/app/store/desktop-controls.effects.spec.ts \
  --include=src/app/store/desktop-controls.reducer.spec.ts
```

**Step 4: Implement**

Use NgRx for committed/draft cross-component state, signals/computed values in
the Settings presenter, `OnPush`, and native Angular template control flow.
Do not add a second form library or UI framework.

**Step 5: Verify and commit**

```bash
npx ng test --watch=false \
  --include=src/app/desktop/apps/settings.app.spec.ts \
  --include=src/app/desktop/preferences.service.spec.ts \
  --include=src/app/desktop/desktop-controls-bridge.service.spec.ts \
  --include=src/app/store/desktop-controls.effects.spec.ts \
  --include=src/app/store/desktop-controls.reducer.spec.ts
npx tsc -p tsconfig.app.json --noEmit
cd ..
git diff --check
git add frontend/src/app/desktop/desktop-controls-bridge.service.ts \
  frontend/src/app/desktop/desktop-controls-bridge.service.spec.ts \
  frontend/src/app/desktop/preferences.service.ts \
  frontend/src/app/desktop/preferences.service.spec.ts \
  frontend/src/app/desktop/apps/settings.app.ts \
  frontend/src/app/desktop/apps/settings.app.spec.ts \
  frontend/src/app/store/desktop-controls.actions.ts \
  frontend/src/app/store/desktop-controls.effects.ts \
  frontend/src/app/store/desktop-controls.effects.spec.ts \
  frontend/src/app/store/desktop-controls.reducer.ts \
  frontend/src/app/store/desktop-controls.reducer.spec.ts \
  frontend/src/app/store/desktop-controls.models.ts
git commit -m "feat: add transactional desktop settings"
```

## Task 6: Move inner desktop restoration to `desktopstate`

**Files:**

- Create: `frontend/src/app/desktop/desktop-state-bridge.service.ts`
- Create: `frontend/src/app/desktop/desktop-state-bridge.service.spec.ts`
- Modify: `frontend/src/app/store/desktop.actions.ts`
- Modify: `frontend/src/app/store/desktop.effects.ts`
- Modify: `frontend/src/app/store/desktop.effects.spec.ts`
- Modify: `frontend/src/app/store/desktop.reducer.ts`
- Modify: `frontend/src/app/store/desktop.reducer.spec.ts`
- Modify: `frontend/src/app/desktop/window-manager.service.ts`
- Modify: `frontend/src/app/app.config.ts` if a dedicated session effect is
  cleaner than adapting `DesktopEffects`

**Step 1: Write failing tests**

Test:

- connected startup loads through the Wails bridge;
- offline startup hydrates deterministic in-memory demo state with no Wails
  call;
- the legacy `lthn.desktop` value is imported once and removed only after a
  successful Medium save;
- saves are debounced for drag/resize;
- discrete mutations request persistence;
- current viewport bounds keep every title bar reachable;
- unknown app IDs and invalid routes are dropped by the existing reducer;
- revision conflict reloads rather than overwrites; and
- unavailable/corrupt persistence falls back to the safe desktop without
  writing over evidence.

**Step 2: Run the red tests**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-state-bridge.service.spec.ts \
  --include=src/app/store/desktop.effects.spec.ts \
  --include=src/app/store/desktop.reducer.spec.ts \
  --include=src/app/desktop/window-manager.service.spec.ts
```

**Step 3: Implement**

Validate Wails payloads at the bridge. Keep the reducer and window manager as
the only UI state owners. Remove connected writes from `StorageService`;
retain it only for explicit demo state and one-time migration.

**Step 4: Verify and commit**

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-state-bridge.service.spec.ts \
  --include=src/app/store/desktop.effects.spec.ts \
  --include=src/app/store/desktop.reducer.spec.ts \
  --include=src/app/desktop/window-manager.service.spec.ts
npx tsc -p tsconfig.app.json --noEmit
cd ..
git diff --check
git add frontend/src/app/desktop/desktop-state-bridge.service.ts \
  frontend/src/app/desktop/desktop-state-bridge.service.spec.ts \
  frontend/src/app/desktop/window-manager.service.ts \
  frontend/src/app/store/desktop.actions.ts \
  frontend/src/app/store/desktop.effects.ts \
  frontend/src/app/store/desktop.effects.spec.ts \
  frontend/src/app/store/desktop.reducer.ts \
  frontend/src/app/store/desktop.reducer.spec.ts \
  frontend/src/app/app.config.ts
git commit -m "feat: restore desktop sessions through go-io"
```

Only add `app.config.ts` if changed.

## Task 7: Add cursor-aware Terminal replay

**Files:**

- Modify: `go/pkg/terminal/session.go`
- Modify: `go/pkg/terminal/session_test.go`
- Modify: `go/pkg/terminal/service.go`
- Modify: `go/pkg/terminal/service_test.go`
- Add or update a Terminal example test for the cursor contract

**Step 1: Write failing tests**

Test:

- live output receives exact monotonic start/end cursors;
- attach after a retained cursor replays only missing bytes;
- attach after an expired cursor sends one reset snapshot;
- attach at the current cursor sends no replay;
- reattach atomically replaces the prior subscriber;
- invalid future cursors reject; and
- existing Write/Resize/Close/List behaviour remains intact.

Avoid a real PTY for cursor unit tests by using the existing in-memory session
fixture.

**Step 2: Run the red tests**

```bash
go test ./go/pkg/terminal -run 'Cursor|Attach|Ring' -count=1
```

**Step 3: Implement**

Add total-byte/ring-base cursor accounting under the existing session lock.
Change the renderer event payload from raw base64 to a typed chunk. Preserve
subscribe-before-replay ordering so no output falls between snapshot and live
subscription.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/terminal -count=1
go test -race ./go/pkg/terminal -count=1
go vet ./go/pkg/terminal
git diff --check
git add go/pkg/terminal
git commit -m "feat: add resumable terminal output"
```

## Task 8: Persist Terminal workspaces and reconnect Angular sessions

**Files:**

- Create:
  `frontend/src/app/desktop/terminal-workspace.models.ts`
- Create:
  `frontend/src/app/desktop/terminal-workspace.service.ts`
- Create:
  `frontend/src/app/desktop/terminal-workspace.service.spec.ts`
- Modify:
  `frontend/src/app/desktop/surfaces/agents/terminal-session.ts`
- Create:
  `frontend/src/app/desktop/surfaces/agents/terminal-session.spec.ts`
- Modify: `frontend/src/app/desktop/surfaces/agents/terminal.ts`
- Modify: `frontend/src/app/desktop/surfaces/agents/terminal.spec.ts`

**Step 1: Write failing tests**

Test:

- cursor chunks write once, duplicates drop, gaps trigger reattach, and reset
  replaces the old xterm buffer;
- reconnecting disables input and connected recovery attaches after the last
  cursor without adding a second Wails listener;
- unknown sessions become visibly exited and can start a fresh shell;
- saved tabs restore order/title/active key from workspace intent;
- persisted payloads omit session ID, absolute cwd, command, environment, and
  output;
- shared agent tabs restore only when `List` reports them;
- tab mutations use a bounded debounce; and
- offline demo mode uses memory only and creates no PTY/listener.

**Step 2: Run the red tests**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/terminal-workspace.service.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal-session.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal.spec.ts \
  --include=src/app/desktop/apps/terminal.app.spec.ts
```

**Step 3: Implement**

Inject `ConnectionManagerService`; use an Angular `effect` tied to
`DestroyRef` for connection transitions. Keep transient session IDs only in
the session component. Translate Files/repository launch intents into
mount/repository references; arbitrary renderer command arrays are not
persisted or accepted by the new workspace bridge.

**Step 4: Verify and commit**

```bash
npx ng test --watch=false \
  --include=src/app/desktop/terminal-workspace.service.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal-session.spec.ts \
  --include=src/app/desktop/surfaces/agents/terminal.spec.ts \
  --include=src/app/desktop/apps/terminal.app.spec.ts
npx tsc -p tsconfig.app.json --noEmit
cd ..
git diff --check
git add frontend/src/app/desktop/terminal-workspace.models.ts \
  frontend/src/app/desktop/terminal-workspace.service.ts \
  frontend/src/app/desktop/terminal-workspace.service.spec.ts \
  frontend/src/app/desktop/surfaces/agents/terminal-session.ts \
  frontend/src/app/desktop/surfaces/agents/terminal-session.spec.ts \
  frontend/src/app/desktop/surfaces/agents/terminal.ts \
  frontend/src/app/desktop/surfaces/agents/terminal.spec.ts
git commit -m "feat: restore resilient terminal workspaces"
```

## Task 9: Centralise Angular host intents

**Files:**

- Create: `frontend/src/app/desktop/desktop-host-intent.service.ts`
- Create:
  `frontend/src/app/desktop/desktop-host-intent.service.spec.ts`
- Modify: `frontend/src/app/deep-link-navigation.service.ts`
- Modify: `frontend/src/app/deep-link-navigation.service.spec.ts`
- Modify: `frontend/src/app/app.config.ts`
- Modify Files/Settings shell launch call sites identified by the failing
  contract tests

**Step 1: Write failing tests**

Test:

- malformed, oversized, raw-path, unknown-kind, unknown-app, unknown-action,
  and unknown-permission payloads are ignored;
- opaque open items route `.lthn` to Settings and ordinary items to Files;
- deep links and tray targets still derive routes from the desktop catalogue;
- notification clicks/actions resolve only server-known intent IDs;
- permission snapshots distinguish policy from host state; and
- offline mode registers no Wails listener.

**Step 2: Run the red tests**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-host-intent.service.spec.ts \
  --include=src/app/deep-link-navigation.service.spec.ts
```

**Step 3: Implement**

Use one injected event-source abstraction so tests and offline mode do not
import the live runtime. Keep compatibility listeners during migration but
route their validated values through the same catalogue functions.

**Step 4: Verify and commit**

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-host-intent.service.spec.ts \
  --include=src/app/deep-link-navigation.service.spec.ts
npx tsc -p tsconfig.app.json --noEmit
cd ..
git diff --check
git add frontend/src/app/desktop/desktop-host-intent.service.ts \
  frontend/src/app/desktop/desktop-host-intent.service.spec.ts \
  frontend/src/app/deep-link-navigation.service.ts \
  frontend/src/app/deep-link-navigation.service.spec.ts \
  frontend/src/app/app.config.ts
git commit -m "feat: route typed native host intents"
```

## Task 10: Complete notifications, permissions, and platform associations

**Files:**

- Extend: `go/pkg/desktop/host_intents.go`
- Extend: `go/pkg/desktop/host_intents_test.go`
- Modify: `go/pkg/desktop/sysevents.go`
- Modify: `go/pkg/permissions/permissions.go`
- Modify or create focused permission tests
- Modify: `build/linux/desktop`
- Modify: `build/linux/lthn.desktop`
- Create: `build/linux/application-x-lethean.xml`
- Modify: `build/linux/nfpm/nfpm.yaml`
- Modify: `build/windows/msix/app_manifest.xml`
- Modify Windows packaging metadata only where the current product identity is
  stale
- Create:
  `scripts/verify-native-integration.test.mjs`
- Modify: `package.json` or the existing root Node-contract entrypoint if
  required to run the new contract

**Step 1: Write failing Go and packaging tests**

Cover:

- bounded notification kinds and action IDs;
- click/action/dismiss host-intent translation;
- verified host permission states and explicit request-only behaviour;
- startup performs no prompt;
- unsupported status remains unsupported/unknown; and
- macOS, Linux, Windows NSIS, and Windows MSIX agree on `lthn`, `.lthn`,
  product executable, and application identity.

**Step 2: Run the red tests**

```bash
go test ./go/pkg/desktop ./go/pkg/permissions \
  -run 'Notification|Permission|HostIntent' -count=1
node --test scripts/verify-native-integration.test.mjs
```

**Step 3: Implement**

Use existing CoreGUI notification/permission capabilities when verifiably
available. Report unsupported instead of inventing platform support. Do not
add mobile manifest claims without a tested Wails mobile handler.

**Step 4: Verify and commit**

```bash
go test ./go/pkg/desktop ./go/pkg/permissions -count=1
go vet ./go/pkg/desktop ./go/pkg/permissions
node --test scripts/verify-native-integration.test.mjs
git diff --check
git add go/pkg/desktop/host_intents.go \
  go/pkg/desktop/host_intents_test.go \
  go/pkg/desktop/sysevents.go \
  go/pkg/permissions \
  build/linux/desktop build/linux/lthn.desktop \
  build/linux/application-x-lethean.xml build/linux/nfpm/nfpm.yaml \
  build/windows/msix/app_manifest.xml \
  scripts/verify-native-integration.test.mjs package.json
git commit -m "feat: complete native desktop intents"
```

Only add files that actually changed.

## Task 11: Update project contracts and remove completed TODOs

**Files:**

- Modify: `AGENTS.md`
- Modify: `TODO.md`
- Modify: `docs/development.md`
- Modify other directly contradicted desktop documentation found by exact
  search

**Step 1: Write or extend contract assertions first**

Add the new state, Terminal, host-intent, demo-mode, and Medium boundary
requirements to the existing convergence/native Node contracts where a
machine-checkable invariant is appropriate.

**Step 2: Update documentation**

Document:

- the `desktopstate` ownership boundary and document paths;
- Settings draft/apply behaviour;
- Terminal cursor/reconnect and non-persisted data;
- browser demo versus connected persistence;
- opaque host-item contract; and
- platform association verification.

Remove only TODO entries genuinely completed by verified code.

**Step 3: Verify and commit**

```bash
node --test scripts/*.test.mjs
git diff --check
git add AGENTS.md TODO.md docs/development.md scripts
git commit -m "docs: record resilient desktop contracts"
```

Stage only changed, intended scripts.

## Task 12: Full verification and delivery

**Step 1: Inspect scope before running gates**

```bash
git status --short
git log --oneline --decorate -15
git diff origin/main...HEAD --stat
git diff origin/main...HEAD --check
```

Confirm `go.work.sum` and `.playwright-mcp/` remain user-owned and uncommitted.

**Step 2: Run Go checks**

```bash
gofmt -l go/
go test ./go/pkg/desktopstate ./go/pkg/appconfig ./go/pkg/terminal \
  ./go/pkg/office/files ./go/pkg/permissions ./go/pkg/desktop \
  ./go/cmd/lthn -count=1
go test -race ./go/pkg/desktopstate ./go/pkg/appconfig ./go/pkg/terminal \
  ./go/pkg/office/files -count=1
go vet ./go/pkg/desktopstate ./go/pkg/appconfig ./go/pkg/terminal \
  ./go/pkg/office/files ./go/pkg/permissions ./go/pkg/desktop \
  ./go/cmd/lthn
go tool wails3 task test:go
```

If the running desktop owns port `9099`, close only that development process
or record the environmental collision; never kill unrelated processes.

**Step 3: Run frontend and contract checks**

```bash
cd frontend
npx ng test --watch=false
npx tsc -p tsconfig.app.json --noEmit
npm run build
cd ..
node --test scripts/*.test.mjs
go tool wails3 task test:frontend
```

Measure fresh coverage and report it as an observation, not an enforced gate.

**Step 4: Run final repository checks**

```bash
git diff --check
go tool wails3 task test
task verify:frontend
```

Run the external compliance audit only as a before/after no-regression
diagnostic and report its known pre-existing backlog honestly.

**Step 5: Review commits and push**

```bash
git status --short --branch
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git push origin main
```

Report exact commits, verification results, any environmental failures, and
the two preserved user-owned worktree items.
