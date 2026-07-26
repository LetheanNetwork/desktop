<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Files `io.Medium` Integration Design

## Goal

Turn Lethean Desktop's Files application into a working, provider-neutral file
browser whose entire data path is backed by CoreGO `io.Medium`.

The Files application must work with sandboxed local storage immediately and
must accept S3, SFTP, WebDAV, SQLite, Store, Cube, Memory, or another Medium
without changing its Angular component contract.

This is both an application boundary and a security boundary. A Files feature
is not complete if it reaches a file through a second local-only path.

## Immutable security invariant

Every operation that observes or changes file-backed state must:

1. resolve a server-registered mount identifier;
2. obtain that mount's `dappco.re/go/io.Medium`;
3. validate a provider-relative path;
4. execute through that Medium; and
5. return a provider-neutral result.

This applies to:

- listing and metadata;
- reads and bounded previews;
- writes and streamed writes;
- directory creation;
- rename, copy, and move;
- trash, restore, and permanent deletion;
- existence and conflict checks;
- recent-file discovery; and
- recursive traversal.

There is no fallback to `os`, `path/filepath`, `syscall`, an unrestricted
`core.Fs`, `core.ReadDir`, `core.DirFS`, `core.Stat`, or `core.ReadFile` inside
the Files data plane. An unavailable Medium or failed boundary check produces a
typed unavailable or permission result. It never falls back to a host path.

Tests and examples seed and inspect data through a Medium as well. A focused
source-contract test prevents the Files package from regaining a raw
filesystem import or Core filesystem convenience call.

## Current mismatch

The repository already directly pins `dappco.re/go/io v0.15.1`, and
`cmd/lthn/app.go` registers the CoreGO I/O service. The package provides the
required pivot:

```go
type Medium interface {
    Read(path string) (string, error)
    Write(path, content string) error
    WriteMode(path, content string, mode fs.FileMode) error
    EnsureDir(path string) error
    Delete(path string) error
    DeleteAll(path string) error
    Rename(oldPath, newPath string) error
    List(path string) ([]fs.DirEntry, error)
    Stat(path string) (fs.FileInfo, error)
    Open(path string) (fs.File, error)
    Create(path string) (io.WriteCloser, error)
    Append(path string) (io.WriteCloser, error)
    ReadStream(path string) (io.ReadCloser, error)
    WriteStream(path string) (io.WriteCloser, error)
    Exists(path string) bool
    IsFile(path string) bool
    IsDir(path string) bool
}
```

The current `go/pkg/office/files` service does not use that boundary. It scans
hard-coded host locations through Core filesystem wrappers, reads absolute
paths, and obtains disk capacity with a direct platform syscall. Its Wails
contract reports saved local locations and recent files rather than browsing a
Medium.

The Angular Files component then reconstructs a shallow pseudo-filesystem from
those local-only rows. This is the seam to replace.

## Upstream boundary gate

The pinned and current upstream local Medium contains an explicit security
warning: its path containment currently validates path components and later
passes the resulting string to an unrestricted filesystem operation. A
filesystem object can theoretically be swapped between those steps, creating a
time-of-check/time-of-use race around a symlink.

Desktop must not paper over this inside `go/pkg/office/files`. The correct
owner is `go-io`, because every CoreGO consumer relies on the same boundary.

Before enabling the local mount in a release:

- add an adversarial regression test to `core/go-io/go/local` that attempts a
  component or symlink swap between validation and use;
- replace validate-then-use with handle-relative, no-follow operations for
  every local Medium method;
- make unsupported platform paths fail closed rather than falling back to the
  old unrestricted operation;
- retain the `io.Medium` interface so other providers and consumers do not
  change;
- release the repaired go-io module; and
- update Lethean Desktop to that version before calling the local Files mount
  production-ready.

The desktop implementation may be developed against Memory and other safe
Mediums while this upstream gate is repaired. It must not add a second
sandboxing implementation or claim that repeated string validation closes the
race.

## Approaches considered

### Registered Medium mounts behind a Files façade — selected

`go/pkg/office/files` owns a closed mount registry. Each mount contains an
opaque ID, display metadata, declared capabilities, and an `io.Medium`.

Angular addresses a mount and a relative path. It never receives provider
credentials, a local root, or an unrestricted absolute path.

This provides one stable desktop contract for local and remote providers while
preserving CoreGO's security boundary.

### Dispatch generic Core I/O actions from Angular

The existing action catalogue is useful for automation, but exposing its
provider-specific option maps directly to the WebView would couple the UI to
backend action names and credential-bearing configuration. It would also make
mount-level capability and consent checks harder to audit.

### Replace the current wrappers with local `io.NewSandboxed` calls only

This would stop the immediate wrapper bypass but retain local location DTOs and
force another frontend and Wails redesign for S3, SFTP, or other media. It does
not establish the product-level pivot around `io.Medium`.

## Go component boundary

`go/pkg/office/files` remains the trusted Wails façade. Its internal shape
becomes:

```go
type Mount struct {
    ID           string
    Name         string
    Kind         string
    Icon         string
    Brand        bool
    Capabilities Capabilities
    Medium       coreio.Medium
}

type Options struct {
    Mounts  []Mount
    Runtime RuntimeMetadata
}

type Service struct {
    core    *core.Core
    mounts  map[string]Mount
    runtime RuntimeMetadata
}
```

`Mount.Medium` is internal and is never serialised. Mount IDs are unique,
stable, and server-owned. Provider kind is display information such as
`local`, `s3`, `sftp`, `webdav`, `cube`, or `memory`; it is not used by the
browser to select a code path.

`NewService(Options)` provides deterministic dependency injection.
`Register(*core.Core)` composes the canonical desktop mounts and returns the
service through the established CoreGO `core.Result` convention.

The first desktop composition registers the current local locations as
separate, narrow sandboxed media rather than one unrestricted home-directory
Medium. Tests register `io.NewMemoryMedium()` instances. A configured remote
provider is another Mount and needs no service or frontend branch.

The existing application-wide I/O service remains the owner of
`~/Lethean/data`. Files does not reuse that Medium to escape its declared root.

## Provider-neutral wire contract

The Wails surface exposes DTOs, not `fs.DirEntry`, `fs.FileInfo`, or provider
SDK types:

```text
FileMount
  id, name, kind, icon, brand, capabilities, optional capacity

FileEntry
  name, relativePath, kind, sizeBytes, modifiedAt, mode, hidden

DirectorySnapshot
  mount, path, breadcrumbs, entries, nextCursor, totalKnown, refreshedAt

FileOperationResult
  status, mountId, path, destination, affectedEntries, conflict, message
```

Initial methods are:

- `ListMounts`;
- `ListDirectory`;
- `Preview`;
- `CreateDirectory`;
- `Rename`;
- `Copy`;
- `Move`;
- `Trash`;
- `ListTrash`;
- `Restore`;
- `Delete`.

All methods return `core.Result` and stable error codes for invalid input,
unknown mount, capability denied, missing entry, conflict, unavailable
provider, partial move, and boundary rejection.

Legacy `ListLocations`, `ListRecent`, and `GetDiskUsage` are migrated off the
Angular path. They may remain as temporary compatibility adapters only when
another verified consumer exists; otherwise they are removed with their
local-only DTOs and tests in the same change.

## Addressing and containment

The WebView supplies only:

- a mount ID selected from `ListMounts`; and
- a slash-normalised relative path within that mount.

The Files service rejects:

- absolute paths;
- empty mount IDs;
- `..` components;
- NUL and bidi/control characters;
- path separators in a single-entry name;
- a destination that resolves to the source; and
- operations on the mount root which would delete, rename, or trash it.

This validation gives clear application errors. It is defence in depth, not a
replacement for Medium containment.

Breadcrumbs are derived from the validated relative path. The host root,
provider endpoint, bucket credentials, SSH details, encryption keys, and
resolved absolute paths never cross Wails.

## Operation semantics

### Listing and preview

`ListDirectory` calls `Medium.List` and uses the returned entries' `Info`
metadata. Results are sorted deterministically, directories first, and bounded
by a server maximum. Cursor state contains no host path or credential.

`Preview` opens `Medium.ReadStream` and consumes a bounded prefix from that
stream. It never opens the same path through another API. Binary or oversized
content returns metadata and an explicit unsupported/truncated state rather
than an unbounded body.

### Create and rename

`CreateDirectory` uses `Medium.EnsureDir`. `Rename` checks destination
conflicts through the same Medium, then calls `Medium.Rename`.

### Copy and move

Single files use `coreio.Copy`. Directories are traversed through
`Medium.List`; destination directories use `EnsureDir`, and leaves use
`coreio.Copy`.

Cross-provider copy receives source and destination mount IDs and otherwise
uses the same algorithm. Move deletes the source only after every destination
write succeeds. A destination success followed by a source-delete failure is
reported as a typed partial move and is never described as atomic.

### Trash, restore, and delete

Trash is a desktop operation built on the selected Medium, not a call to an OS
trash API. A writable mount reserves an internal trash namespace. Trashing
renames the entry into that namespace and records only the receipt needed to
restore its original relative path.

`go-store` may persist desktop runtime metadata such as favourites, recent
locations, and trash receipts. It does not read or write user file content.
The only exception is when a go-io Store Medium is itself the selected file
provider; the Files service still talks exclusively through `io.Medium`.

Restore checks for destination conflict before renaming the entry back.
Permanent deletion is separate, explicit, and requires the caller to confirm
recursive deletion for a non-empty directory.

### Capacity

The current `diskusage_*.go` syscall path is removed from the Files data plane.
Capacity is optional provider metadata and may be shown only when the selected
Medium exposes it through an audited go-io capability. Otherwise the live UI
shows `Capacity unavailable`; it does not substitute a demo disk value.

## Capability model

Each mount declares the operations it permits:

```text
list, preview, createDirectory, write, rename,
copyFrom, copyTo, move, trash, restore, delete
```

The server enforces capabilities before invoking the Medium. Angular uses the
same flags to disable or omit invalid actions, but frontend state is never the
authorisation boundary.

Read-only providers remain useful for browsing and cross-provider copy-out.
Credentials and provider construction remain in trusted Go composition.

Adding provider-credential forms or automatically discovering remote accounts
is a separate design because those flows cross the existing key and consent
boundaries.

## Angular component boundary

The Angular 22 application uses provider-neutral readonly models:

- `FilesViewState`;
- `FileMountView`;
- `FileEntryView`;
- `FilesBreadcrumb`;
- `FilesCapabilities`;
- `FilesActionIntent`; and
- `FilesOperationFeedback`.

Pure functions create demo state and reconcile a live directory snapshot.
Provider-specific logic is not permitted in a component or mapper.

Extract standalone, `OnPush` views under `apps/files/`:

- `files-sidebar.view.ts` — favourites and registered mounts;
- `files-toolbar.view.ts` — up/home navigation, breadcrumbs, refresh, and
  grid/list selection;
- `files-browser.view.ts` — empty, grid, and list presentations plus selection;
- `files-status.view.ts` — truthful data state, counts, provider, and optional
  capacity; and
- `files-operation-dialog.view.ts` — typed confirmation and conflict feedback
  for mutations.

Views use signal inputs and function-based outputs. They do not inject Wails,
`DesktopLiveDataService`, `WindowManagerService`, or a provider client.

`FilesApp` remains the container and owns:

- the current mount/path and view mode;
- live bridge calls and operation progress;
- WindowManager route persistence;
- selection and confirmation orchestration;
- WebMCP registration; and
- refresh after successful mutations or Files events.

A dedicated `DesktopFilesBridgeService` owns Files Wails method names and
response validation. The general `DesktopLiveDataService` no longer has to
reconstruct a filesystem from location/recent/disk calls.

## Navigation state

`win.sub` stores a reversible, non-secret Files location token containing a
mount ID and provider-relative path. Encoding and decoding are pure functions.
Invalid or unavailable tokens fall back to the virtual Files home, never to an
absolute host path.

The virtual home lists registered mounts and recent runtime metadata. Selecting
a mount opens its root. Existing grid/list state remains in `win.systab`.

## Demo and connected modes

Explicit offline transport remains a deterministic product demo:

- it never invokes a Wails method;
- it uses typed demo mounts, paths, entries, operation outcomes, and trash
  receipts;
- demo values remain visibly labelled;
- safe scripted operations mutate only in-memory browser demo state; and
- no offline preview can execute a local command or touch a host file.

Connected mode displays only Medium-backed live values. If a provider fails,
the last successful state may remain visible with a stale/unavailable label,
but fixture values are not silently presented as live.

## Events and refresh

Every successful mutation emits a Files event containing mount ID, affected
relative paths, operation, and timestamp. No host root or credentials are
included.

The active Files window refreshes on matching events. Provider-native watch
integration can later emit the same event shape. Bounded polling is a fallback
for providers without watch support, not a second file-access path.

## Error handling

Errors remain actionable without leaking host details:

- permission and boundary failures identify the mount and relative path;
- provider errors are mapped to stable codes and calm British-English copy;
- raw endpoint responses, absolute paths, credentials, and encryption details
  stay in Go logs;
- conflict results preserve both source and destination entries;
- partial cross-provider operations report exactly what completed; and
- a failed refresh never changes `unavailable` into `demo`.

## Testing

Use red-green tests at each boundary.

### go-io security gate

- adversarial symlink/component swap tests fail against validate-then-use;
- every local Medium operation uses the repaired handle-relative boundary;
- unsupported platform implementations fail closed;
- existing Memory, S3, SFTP, WebDAV, Cube, SQLite, and Store contracts remain
  unchanged.

### Go Files service

- Good tests browse, preview, create, rename, copy, move, trash, restore, and
  delete through `io.NewMemoryMedium`;
- cross-Medium tests prove transfer without provider-specific branches;
- Bad tests cover unknown mounts, missing entries, read-only capabilities,
  destination conflicts, and failed providers;
- Ugly tests cover absolute paths, traversal, controls, root mutation,
  recursive delete confirmation, partial move, and bounded preview/listing;
- local integration uses `t.TempDir()` and seeds exclusively through
  `io.NewSandboxed`;
- no test writes to the real `~/Lethean` tree;
- source-contract tests reject raw filesystem imports and bypass calls; and
- runnable examples use Memory Mediums.

### Angular

- pure state tests cover demo and live directory reconciliation;
- each extracted view renders its input and emits typed intents without Wails;
- the container makes no live call in offline mode;
- connected mode navigates mounts and relative paths correctly;
- operation progress, conflicts, errors, and refresh are truthful;
- grid/list, breadcrumbs, empty state, sidebar, status, localisation, and
  WebMCP behaviour remain present; and
- bridge tests reject malformed or path-leaking payloads.

Final verification runs focused Go and Angular tests, the frontend confidence
gate, the Angular production build, `go vet` for changed Go packages, and
`git diff --check`.

## Documentation corrections

Update `TODO.md` and the capability matrix language from “add a filesystem
capability” to “wire the existing go-io capability through the desktop Files
façade”. Remaining TODOs must identify missing desktop composition, provider
configuration, or UI integration rather than describing implemented CoreGO
features as aspirational.

Update `AGENTS.md` with the invariant that `io.Medium` is the only permitted
file data path for product services. `go-store` is runtime metadata or an
implementation behind a selected Medium, never an alternate Files data path.

## Deferred work

This tranche does not add provider credential forms, auto-discover remote
accounts, expose raw local roots, implement provider SDK calls in Angular, or
generalise the Files façade into a plugin permission system.

Those capabilities can register or authorise mounts later without changing the
provider-neutral browser contract or weakening the `io.Medium` pivot.
