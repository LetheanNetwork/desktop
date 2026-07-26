<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Files `io.Medium` Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:executing-plans to implement this plan task-by-task in the current
> session. Steps use checkbox (`- [ ]`) syntax for tracking. The owner has
> explicitly prohibited sub-agents for this project.

**Goal:** Replace both host-path Files bindings with one tested,
provider-neutral Files application whose content, metadata, and persistent
runtime state always flow through registered `dappco.re/go/io.Medium`
instances.

**Architecture:** `go/pkg/office/files` becomes the sole Wails Files façade and
addresses closed, server-registered mounts by opaque ID plus relative path.
The façade is first built and tested with Memory media, then local mounts are
enabled only after `go-io/local` is repaired around Go 1.26's handle-relative
`os.Root` and released as `go/v0.15.2`. Angular owns provider-neutral view
state, offline demo state, navigation, and presentation; it never receives a
host root or selects a provider implementation.

**Tech Stack:** Go 1.26, CoreGO `core.Result`, `dappco.re/go/io`,
`io.Medium`, `os.Root`, Wails 3 alpha, Angular 22 standalone components,
TypeScript 6, signals, Vitest/TestBed, Lit custom elements, npm.

## Global Constraints

- Execute inline on `main`; do not dispatch sub-agents or create a worktree.
- Every Files observation or mutation, including runtime metadata, must enter
  through a registered `io.Medium`. There is no raw-path fallback.
- `go/pkg/office/files` must not import `os`, `path/filepath`, or `syscall`, or
  call `core.ReadFile`, `core.WriteFile`, `core.ReadDir`, `core.DirFS`,
  `core.Stat`, or an unrestricted `core.Fs`.
- Provider implementations may use platform APIs internally to implement the
  Medium contract; product callers may only see `io.Medium`.
- Do not enable a production local mount until Desktop pins
  `dappco.re/go/io v0.15.2` containing the `os.Root` repair.
- Do not use `Medium.Exists`, `IsFile`, or `IsDir` for security, conflict, or
  authorisation decisions because their boolean contract cannot distinguish an
  unavailable provider from a missing path. Use `Stat` and preserve errors.
- Wails inputs contain only a registered mount ID and slash-normalised
  relative path. Absolute paths, roots, endpoints, credentials, and encryption
  material never cross the renderer boundary.
- Preserve the complete Files UX: sidebar, Home, nested navigation, Up,
  breadcrumbs, grid/list views, empty state, item counts, data-state badge,
  capacity label, read-only WebMCP tools, and the existing labelled demo.
- Explicit offline transport performs no Wails calls, registers no Wails event
  listener, and mutates only an in-memory demo catalogue.
- Do not expose mutating Files WebMCP tools in this tranche.
- Use British English and retain EUPL-1.2 headers.
- Use red-green TDD. Go tests seed and inspect application data through Memory
  media; the upstream provider's adversarial tests may use OS primitives to
  construct attacks around the provider boundary.
- Local integration tests use `t.TempDir()` and never write to the real
  `~/Lethean/` tree.
- Preserve non-GUI CLI behaviour and the Angular production output contract.
- Leave the user-owned `.playwright-mcp/` directory untouched.
- Existing raw filesystem use elsewhere in the repository is tracked as a
  separate migration backlog. This tranche must not claim global compliance,
  but it must add a zero-bypass guard for the Files scope and prohibit new
  bypasses immediately.

---

## File and responsibility map

### Upstream `/Users/snider/Code/core/go-io`

- Modify `go/local/medium.go` — anchor every local Medium operation to one
  `*os.Root`; keep legacy path normalisation without validate-then-use.
- Delete `go/local/medium_link.go` — remove the path-string symlink resolver.
- Delete `go/local/medium_link_test.go` — replace helper tests with observable
  containment tests.
- Create `go/local/medium_root_test.go` — deterministic component/symlink swap,
  internal-link, protected-root, and lifecycle contracts.
- Modify `go/local/medium_test.go` — construct/close rooted media and preserve
  the complete Medium Good/Bad/Ugly suite.
- Modify `go/service.go` — close closable sandboxed media during service
  shutdown.
- Modify `go/service_test.go` — prove shutdown releases the local Root.

### Lethean Desktop Go

- Rewrite `go/pkg/office/files/types.go` — provider-neutral wire types, inputs,
  capabilities, limits, error codes, and operation results.
- Rewrite `go/pkg/office/files/service.go` — canonical `Options`, `Service`,
  mount registry, Core registration, locking, and error helpers.
- Create `go/pkg/office/files/default_mounts.go` — fail-closed registration
  seam which Task 8 replaces with audited runtime and local-Mount composition.
- Create `go/pkg/office/files/path.go` — mount-ID, relative-path, single-name,
  breadcrumb, and internal-namespace validation.
- Create `go/pkg/office/files/listing.go` — mount catalogue and bounded
  directory snapshots.
- Create `go/pkg/office/files/preview.go` — bounded text/binary preview through
  `ReadStream`.
- Create `go/pkg/office/files/runtime.go` — favourites, recents, and trash
  receipts persisted as a document through a runtime Medium.
- Create `go/pkg/office/files/mutations.go` — create-directory, rename,
  capabilities, conflicts, and mutation events.
- Create `go/pkg/office/files/transfer.go` — bounded preflight, streaming copy,
  same/cross-Medium move, link rejection, staging, and partial results.
- Create `go/pkg/office/files/trash.go` — owned internal namespace, trash,
  restore, and confirmed permanent deletion.
- Rewrite `go/pkg/office/files/wails.go` — thin `core.Result` methods over the
  service operations.
- Rewrite `go/pkg/office/files/events.go` — typed Core ACTION event.
- Rewrite `go/pkg/office/files/files_example_test.go` — runnable Memory-Medium
  examples.
- Rewrite `go/pkg/office/files/files_test.go` and
  `go/pkg/office/files/wails_test.go` — registry, path, wire, operation, and
  Wails tests.
- Create `go/pkg/office/files/listing_test.go`,
  `preview_test.go`, `runtime_test.go`, `mutations_test.go`,
  `transfer_test.go`, `trash_test.go`, and `medium_boundary_test.go`.
- Delete `go/pkg/office/files/diskusage_unix.go` and
  `go/pkg/office/files/diskusage_windows.go`.
- Delete `go/pkg/files/files.go` and `go/pkg/files/files_test.go`.
- Modify `go/cmd/lthn/app.go` — compose the canonical Files service and update
  the binding comments.
- Modify `go/pkg/desktop/desktop.go` — bind only the Core-registered Office
  Files service.
- Create `go/pkg/desktop/files_events.go` and
  `go/pkg/desktop/files_events_test.go` — relay typed Core events as
  `lthn:files:changed`.
- Modify `go/pkg/terminal/service.go` — remove the retired `pkg/files`
  trust-model comparison.
- Modify `go/go.mod` and `go/go.sum` — pin the repaired go-io release.

### Angular

- Create
  `frontend-ng/src/app/desktop/apps/files/files-view.models.ts` — readonly
  provider-neutral view and action types.
- Create
  `frontend-ng/src/app/desktop/apps/files/files-demo.data.ts` — the complete
  existing Files fixture represented as mounts and relative paths.
- Create
  `frontend-ng/src/app/desktop/apps/files/files-demo.store.ts` — deterministic
  in-memory demo browsing and safe demo mutations.
- Create
  `frontend-ng/src/app/desktop/apps/files/files-view-state.ts` and its spec —
  navigation tokens, pure mapping, icons, labels, and state reconciliation.
- Create
  `frontend-ng/src/app/desktop/desktop-files-bridge.service.ts` and its spec —
  Wails method names, offline guard, strict response parsing, and event
  subscription.
- Create `frontend-ng/src/app/desktop/apps/files/files-sidebar.view.ts`,
  `files-toolbar.view.ts`, `files-browser.view.ts`,
  `files-status.view.ts`, `files-preview.view.ts`, and
  `files-operation-dialog.view.ts` — standalone `OnPush` presentation views.
- Create
  `frontend-ng/src/app/desktop/apps/files/files-views.spec.ts` — isolated view
  input/output contracts.
- Create `frontend-ng/src/app/desktop/apps/files/files.app.scss` — move the
  existing `.fb*` visual contract out of the desktop shell stylesheet.
- Rewrite `frontend-ng/src/app/desktop/apps/files.app.ts` and its spec — small
  route/container owning live/demo orchestration, navigation, preview,
  mutations, events, and WebMCP.
- Modify `frontend-ng/src/app/desktop/apps/app-mcp.spec.ts` — retain all three
  existing Files tools against mount/path navigation.
- Modify `frontend-ng/src/app/desktop/desktop-live-data.service.ts` and its
  spec — remove the retired shallow Files aggregate.
- Modify `frontend-ng/src/app/desktop/desktop.data.ts` — remove the Files-only
  `FS`, `FsNode`, and `DesktopData.fs` fixture.
- Modify `frontend-ng/src/app/desktop/desktop.component.scss` — remove the
  relocated `.fb*` block only.
- Rewrite `frontend-ng/src/app/desktop/surfaces/office/files.ts` — reuse the
  canonical `FilesApp` instead of retaining a second fixture view.
- Modify `frontend-ng/src/app/desktop/surfaces/agents/code.ts` — refer to
  mount-relative `Files.Preview`, never `Files.Read`.
- Modify `frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts` — prove
  the Office route resolves the canonical Files UI.

### Documentation and audit

- Modify `AGENTS.md` — add the repository-wide `io.Medium` invariant.
- Modify `TODO.md` — mark implemented Files contracts and describe remaining
  provider/watch/search work accurately.
- Modify
  `docs/superpowers/specs/2026-07-26-files-go-io-design.md` — clarify that the
  pinned go-store may only be used when every persistent byte is transported
  through an audited Medium; the initial runtime document uses go-io directly.
- Create `docs/security/io-medium-audit.md` — reproducible inventory and staged
  migration record for pre-existing non-Files bypasses.

---

### Task 1: Provider-neutral mount, path, and error contracts

**Files:**

- Rewrite: `go/pkg/office/files/types.go`
- Rewrite: `go/pkg/office/files/service.go`
- Create: `go/pkg/office/files/default_mounts.go`
- Create: `go/pkg/office/files/path.go`
- Rewrite: `go/pkg/office/files/files_test.go`

**Interfaces:**

- Consumes: `core.Core` and `coreio.Medium`.
- Produces:
  `Mount`, `Capabilities`, `Limits`, `Options`,
  `RuntimeMetadata`, `RuntimeSnapshot`, `Favourite`, `Recent`, `TrashReceipt`,
  `NewService(Options) *Service`,
  `(*Service).Register(*core.Core) core.Result`,
  `Register(*core.Core) core.Result`,
  `normaliseRelativePath(string, bool) (string, error)`,
  `validateEntryName(string) error`, and
  `(*Service).mount(string) (Mount, error)`.

- [ ] **Step 1: Write failing registry and path tests**

Replace the old location-format tests with focused contracts:

```go
func TestService_RegisterMounts_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{
		Mounts: []Mount{{
			ID: "documents", Name: "Documents", Kind: "memory",
			Capabilities: ReadWriteCapabilities(), Medium: medium,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
		Limits:  DefaultLimits(),
	})

	result := service.Register(core.New())

	core.AssertTrue(t, result.OK)
	got, err := service.mount("documents")
	core.AssertNoError(t, err)
	core.AssertSame(t, medium, got.Medium.(*coreio.MemoryMedium))
}

func TestService_RegisterMounts_BadDuplicateID(t *core.T) {
	service := NewService(Options{
		Mounts: []Mount{
			{
				ID: "documents", Name: "A", Medium: coreio.NewMemoryMedium(),
				ContainmentAudited: true,
			},
			{
				ID: "documents", Name: "B", Medium: coreio.NewMemoryMedium(),
				ContainmentAudited: true,
			},
		},
		Runtime: &stubRuntimeMetadata{},
	})

	result := service.Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorInvalidMount))
}

func TestNormaliseRelativePath_Ugly(t *core.T) {
	for _, input := range []string{
		"/etc/passwd", `C:\Windows\win.ini`, "../secret", "a/../../secret",
		"a\\b", "a/\x00b", "a/\u202eb", ".lthn-files/trash/x",
	} {
		_, err := normaliseRelativePath(input, false)
		core.AssertError(t, err, input)
	}
}

func TestNormaliseRelativePath_RootGood(t *core.T) {
	got, err := normaliseRelativePath("", true)
	core.AssertNoError(t, err)
	core.AssertEqual(t, "", got)
}
```

Define `stubRuntimeMetadata` in `files_test.go` with `Load` and `Save` methods
over an in-memory `RuntimeSnapshot`; Task 3 replaces it in product use, but the
test helper lets Tasks 1 and 2 compile independently. Add
`memoryMount` and `registeredMemoryService` helpers here as well.
`memoryMount` always sets `Kind: "memory"`,
`ContainmentAudited: true`, and the supplied capabilities:

```go
type stubRuntimeMetadata struct {
	snapshot RuntimeSnapshot
	loadErr  error
	saveErr  error
}

func (runtime *stubRuntimeMetadata) Load() (RuntimeSnapshot, error) {
	if runtime.loadErr != nil {
		return RuntimeSnapshot{}, runtime.loadErr
	}
	return normaliseRuntimeSnapshot(runtime.snapshot), nil
}

func (runtime *stubRuntimeMetadata) Save(snapshot RuntimeSnapshot) error {
	if runtime.saveErr != nil {
		return runtime.saveErr
	}
	runtime.snapshot = normaliseRuntimeSnapshot(snapshot)
	return nil
}

func registeredMemoryService(
	t *core.T,
	id string,
	medium coreio.Medium,
	capabilities Capabilities,
) *Service {
	service := NewService(Options{
		Mounts: []Mount{memoryMount(id, medium, capabilities)},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)
	return service
}
```

- [ ] **Step 2: Run the package test and observe the missing-contract failure**

Run:

```bash
go test ./go/pkg/office/files -run \
  'Test(Service_RegisterMounts|NormaliseRelativePath)' -count=1
```

Expected: FAIL because the new mount, runtime, limits, and path contracts do
not exist.

- [ ] **Step 3: Define the exact wire and service types**

Use these public shapes in `types.go`:

```go
type EntryKind string

const (
	EntryFile      EntryKind = "file"
	EntryDirectory EntryKind = "directory"
	EntryLink      EntryKind = "link"
	EntryOther     EntryKind = "other"
)

type ErrorCode string

const (
	ErrorInvalidInput       ErrorCode = "files.invalid_input"
	ErrorInvalidMount       ErrorCode = "files.invalid_mount"
	ErrorBoundaryRejected   ErrorCode = "files.boundary_rejected"
	ErrorCapabilityDenied   ErrorCode = "files.capability_denied"
	ErrorMissingEntry       ErrorCode = "files.missing_entry"
	ErrorConflict           ErrorCode = "files.conflict"
	ErrorProviderUnavailable ErrorCode = "files.provider_unavailable"
	ErrorLimitExceeded      ErrorCode = "files.limit_exceeded"
	ErrorUnsupportedEntry   ErrorCode = "files.unsupported_entry"
	ErrorPartialMove        ErrorCode = "files.partial_move"
)

type Capabilities struct {
	List            bool `json:"list"`
	Preview         bool `json:"preview"`
	CreateDirectory bool `json:"createDirectory"`
	Write           bool `json:"write"`
	Rename          bool `json:"rename"`
	CopyFrom        bool `json:"copyFrom"`
	CopyTo          bool `json:"copyTo"`
	Move            bool `json:"move"`
	Trash           bool `json:"trash"`
	Restore         bool `json:"restore"`
	Delete          bool `json:"delete"`
}

func ReadWriteCapabilities() Capabilities {
	return Capabilities{
		List: true, Preview: true, CreateDirectory: true, Write: true,
		Rename: true, CopyFrom: true, CopyTo: true, Move: true,
		Trash: true, Restore: true, Delete: true,
	}
}

type Limits struct {
	MaxListEntries    int
	MaxPreviewBytes   int64
	MaxRecursiveDepth int
	MaxRecursiveItems int
	MaxTransferBytes  int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxListEntries: 2_000, MaxPreviewBytes: 512 * 1024,
		MaxRecursiveDepth: 64, MaxRecursiveItems: 100_000,
		MaxTransferBytes: 64 * 1024 * 1024 * 1024,
	}
}

type Mount struct {
	ID                 string        `json:"-"`
	Name               string        `json:"-"`
	Kind               string        `json:"-"`
	Icon               string        `json:"-"`
	Brand              bool          `json:"-"`
	Capabilities       Capabilities  `json:"-"`
	Medium             coreio.Medium `json:"-"`
	ContainmentAudited bool          `json:"-"`
}

type Options struct {
	Mounts  []Mount
	Runtime RuntimeMetadata
	Limits  Limits
	Core    *core.Core
}

type RuntimeMetadata interface {
	Load() (RuntimeSnapshot, error)
	Save(RuntimeSnapshot) error
}

type RuntimeSnapshot struct {
	Version    int            `json:"version"`
	Favourites []Favourite    `json:"favourites"`
	Recent     []Recent       `json:"recent"`
	Trash      []TrashReceipt `json:"trash"`
}

type Favourite struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

type Recent struct {
	MountID string    `json:"mountId"`
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Kind    EntryKind `json:"kind"`
	OpenedAt string   `json:"openedAt"`
}

type TrashReceipt struct {
	ID           string `json:"id"`
	MountID      string `json:"mountId"`
	OriginalPath string `json:"originalPath"`
	TrashPath    string `json:"trashPath"`
	TrashedAt    string `json:"trashedAt"`
}
```

Define `Failure` as an `error` carrying `Code`, `MountID`, relative `Path`,
calm `Message`, and an unexported cause. Its `Error()` string starts with the
stable code and never includes a root or provider credential:

```go
func (failure *Failure) Error() string {
	if failure == nil {
		return string(ErrorProviderUnavailable)
	}
	if failure.Message == "" {
		return string(failure.Code)
	}
	return core.Concat(string(failure.Code), ": ", failure.Message)
}

func (failure *Failure) Unwrap() error { return failure.cause }
```

In `service.go`, copy the input mounts, default zero limits, initialise one
mutex per mount, and keep `RuntimeMetadata` mandatory. `Register` rejects nil
media, invalid/duplicate IDs, empty names, a mount whose trusted composer did
not set `ContainmentAudited`, and a nil runtime before attaching the Core.
Memory media in tests are audited by construction; Task 9 marks local media
audited only after the v0.15.2 `os.Root` release is pinned:

```go
type Service struct {
	core          *core.Core
	pendingMounts []Mount
	mounts        map[string]Mount
	runtime       RuntimeMetadata
	limits        Limits
	locks         map[string]*sync.Mutex
}

func NewService(options Options) *Service {
	limits := options.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	return &Service{
		core: options.Core,
		pendingMounts: append([]Mount(nil), options.Mounts...),
		mounts: make(map[string]Mount),
		runtime: options.Runtime, limits: limits,
		locks: make(map[string]*sync.Mutex),
	}
}

func (service *Service) Register(c *core.Core) core.Result {
	if service == nil || service.runtime == nil {
		return core.Fail(newFailure(ErrorInvalidInput, "", "", "runtime Medium is required", nil))
	}
	service.core = c
	for _, mount := range service.pendingMounts {
		if err := service.addMount(mount); err != nil {
			return core.Fail(err)
		}
	}
	return core.Ok(service)
}
```

Add a pure `normaliseRuntimeSnapshot` helper in `types.go` which defaults
`Version` to `1` and allocates empty non-nil slices. Task 3 adds a
`validateAndNormaliseRuntimeSnapshot` error-returning helper which first calls
this structural helper, then applies validation, de-duplication, and limits.

Store a copied `pendingMounts []Mount` in `Service`; do not retain the caller's
slice. Provide the canonical free function:

```go
func Register(c *core.Core) core.Result {
	optionsResult := DefaultOptions(c)
	if !optionsResult.OK {
		return optionsResult
	}
	options, ok := optionsResult.Value.(Options)
	if !ok {
		return core.Fail(newFailure(
			ErrorInvalidInput, "", "", "default Files options are invalid", nil,
		))
	}
	return NewService(options).Register(c)
}
```

`default_mounts.go` declares `DefaultOptions(*core.Core) core.Result` as a
normal function returning
`files.invalid_input: default Files mounts are not composed`. Task 8 replaces
that temporary fail-closed body before any native binding uses it. Do not use a
mutable package-level function variable for this security seam.

- [ ] **Step 4: Implement pure path validation**

In `path.go`, use provider-style `/` semantics, never host `filepath`
semantics:

```go
const internalNamespace = ".lthn-files"

func normaliseRelativePath(raw string, allowRoot bool) (string, error) {
	if raw == "" {
		if allowRoot {
			return "", nil
		}
		return "", newFailure(ErrorInvalidInput, "", "", "path is required", nil)
	}
	if core.HasPrefix(raw, "/") || core.Contains(raw, "\\") ||
		(len(raw) > 1 && raw[1] == ':') {
		return "", newFailure(ErrorBoundaryRejected, "", "", "absolute paths are not accepted", nil)
	}
	parts := core.Split(raw, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", newFailure(ErrorBoundaryRejected, "", "", "path traversal is not accepted", nil)
		}
		if hasUnsafePathRune(part) {
			return "", newFailure(ErrorBoundaryRejected, "", "", "path contains a control character", nil)
		}
		clean = append(clean, part)
	}
	if len(clean) > 0 && clean[0] == internalNamespace {
		return "", newFailure(ErrorBoundaryRejected, "", "", "internal Files paths are not addressable", nil)
	}
	return core.Join("/", clean...), nil
}
```

`hasUnsafePathRune` rejects C0/C1 controls, U+200E/U+200F, U+202A–U+202E,
and U+2066–U+2069. `validateEntryName` additionally rejects `/`, `\`, `.`,
and `..`. Mount IDs accept only lower-case ASCII letters, digits, and `-`, with
a maximum of 64 bytes.

- [ ] **Step 5: Run the focused package test**

Run:

```bash
go test ./go/pkg/office/files -run \
  'Test(Service_RegisterMounts|NormaliseRelativePath)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the foundational contract**

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/service.go \
  go/pkg/office/files/default_mounts.go \
  go/pkg/office/files/path.go \
  go/pkg/office/files/files_test.go
git commit -m "feat(files): define Medium mount boundary"
```

---

### Task 2: Medium-backed mount catalogue, directory listing, and preview

**Files:**

- Create: `go/pkg/office/files/listing.go`
- Create: `go/pkg/office/files/listing_test.go`
- Create: `go/pkg/office/files/preview.go`
- Create: `go/pkg/office/files/preview_test.go`
- Rewrite: `go/pkg/office/files/wails.go`
- Rewrite: `go/pkg/office/files/wails_test.go`

**Interfaces:**

- Consumes: Task 1's `Service`, `Mount`, `Capabilities`, path validators, and
  limits.
- Produces:
  `ListMounts() core.Result`,
  `ListDirectory(ListDirectoryInput) core.Result`, and
  `Preview(PreviewInput) core.Result`.

- [ ] **Step 1: Write failing list and preview tests**

```go
func TestService_ListDirectory_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("notes/readme.md", "hello\nworld"))
	core.RequireNoError(t, medium.Write(".hidden", "quiet"))
	core.RequireNoError(t, medium.EnsureDir("empty"))
	service := registeredMemoryService(t, "documents", medium, ReadWriteCapabilities())

	result := service.ListDirectory(ListDirectoryInput{
		MountID: "documents", Path: "", Limit: 100,
	})

	core.RequireTrue(t, result.OK)
	snapshot := result.Value.(DirectorySnapshot)
	core.AssertEqual(t, "documents", snapshot.Mount.ID)
	core.AssertEqual(t, "", snapshot.Path)
	core.AssertEqual(t, EntryDirectory, snapshot.Entries[0].Kind)
	core.AssertEqual(t, "empty", snapshot.Entries[0].Name)
	core.AssertEqual(t, EntryDirectory, snapshot.Entries[1].Kind)
	core.AssertEqual(t, "notes", snapshot.Entries[1].RelativePath)
	core.AssertEqual(t, ".hidden", snapshot.Entries[2].Name)
	core.AssertTrue(t, snapshot.Entries[2].Hidden)
}

func TestService_ListDirectory_BadProviderError(t *core.T) {
	service := registeredMemoryService(
		t, "documents", &failingMedium{Medium: coreio.NewMemoryMedium(), listErr: fs.ErrPermission},
		ReadWriteCapabilities(),
	)

	result := service.ListDirectory(ListDirectoryInput{MountID: "documents"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
}

func TestService_Preview_UglyBounded(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("large.txt", core.Repeat("x", 33)))
	service := registeredMemoryService(t, "documents", medium, ReadWriteCapabilities())
	service.limits.MaxPreviewBytes = 32

	result := service.Preview(PreviewInput{MountID: "documents", Path: "large.txt"})

	core.RequireTrue(t, result.OK)
	preview := result.Value.(FilePreview)
	core.AssertEqual(t, int64(32), preview.BytesRead)
	core.AssertTrue(t, preview.Truncated)
	core.AssertEqual(t, "documents", preview.MountID)
	core.AssertEqual(t, "large.txt", preview.RelativePath)
}

func TestService_Preview_BadBinary(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("image.bin", "png\x00payload"))
	service := registeredMemoryService(t, "documents", medium, ReadWriteCapabilities())

	result := service.Preview(PreviewInput{MountID: "documents", Path: "image.bin"})

	core.RequireTrue(t, result.OK)
	preview := result.Value.(FilePreview)
	core.AssertTrue(t, preview.Binary)
	core.AssertEqual(t, "", preview.Content)
}
```

Create `failingMedium` in `wails_test.go` as a complete test-only
`coreio.Medium` decorator. Every method delegates to `Medium` except the
explicit injected error fields, so later tasks reuse it without inventing
another fake.

- [ ] **Step 2: Run the new tests and observe missing methods**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(ListDirectory|Preview)' -count=1
```

Expected: FAIL because the DTOs and methods do not exist.

- [ ] **Step 3: Define the provider-neutral read DTOs**

Add these types to `types.go`:

```go
type FileMount struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Icon         string       `json:"icon"`
	Brand        bool         `json:"brand,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
	Capacity     *Capacity    `json:"capacity,omitempty"`
}

type Capacity struct {
	FreeBytes  int64 `json:"freeBytes"`
	TotalBytes int64 `json:"totalBytes"`
}

type FileEntry struct {
	Name         string    `json:"name"`
	RelativePath string    `json:"relativePath"`
	Kind         EntryKind `json:"kind"`
	SizeBytes    int64     `json:"sizeBytes"`
	ModifiedAt   string    `json:"modifiedAt"`
	Mode         uint32    `json:"mode"`
	Hidden       bool      `json:"hidden"`
}

type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type MountCatalogue struct {
	Mounts     []FileMount `json:"mounts"`
	Favourites []Favourite `json:"favourites"`
	Recent     []Recent    `json:"recent"`
}

type ListDirectoryInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type DirectorySnapshot struct {
	Mount       FileMount   `json:"mount"`
	Path        string      `json:"path"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	Entries     []FileEntry `json:"entries"`
	NextCursor  string      `json:"nextCursor,omitempty"`
	TotalKnown  int         `json:"totalKnown"`
	RefreshedAt string      `json:"refreshedAt"`
}

type PreviewInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

type FilePreview struct {
	MountID     string `json:"mountId"`
	RelativePath string `json:"relativePath"`
	Name        string `json:"name"`
	Content     string `json:"content,omitempty"`
	MIME        string `json:"mime"`
	BytesRead   int64  `json:"bytesRead"`
	SizeBytes   int64  `json:"sizeBytes"`
	Lines       int    `json:"lines"`
	Truncated   bool   `json:"truncated"`
	Binary      bool   `json:"binary"`
}
```

`Favourite` and `Recent` were declared with the runtime interface in Task 1.
`ListMounts` loads the injected runtime and returns empty non-nil slices when
there are no saved rows.

- [ ] **Step 4: Implement deterministic bounded listing**

`ListDirectory` performs this exact order:

1. resolve the registered mount and require `Capabilities.List`;
2. validate the relative path;
3. parse the decimal cursor and clamp `Limit` to
   `1..service.limits.MaxListEntries`, defaulting to 200;
4. call only `mount.Medium.List(path)`;
5. call `entry.Info()` for returned entries and fail the whole snapshot if a
   provider metadata read fails;
6. map `fs.ModeSymlink` to `EntryLink`, directories to `EntryDirectory`,
   regular files to `EntryFile`, and everything else to `EntryOther`;
7. sort directories first, then case-insensitive name, then exact name;
8. slice the requested page and return the next decimal offset; and
9. derive breadcrumbs from the validated relative path and stamp
   `core.Now().UTC().Format(time.RFC3339Nano)`.

Never call `Exists`, `IsFile`, or `IsDir`. Do not hide dotfiles; mark them with
`Hidden: true` so Angular can decide how to display them. Exclude only the
server-owned first component `.lthn-files`.

- [ ] **Step 5: Implement bounded streaming preview**

`Preview` requires `Capabilities.Preview`, uses `Stat` to reject directories
and link/other modes, then reads exactly `MaxPreviewBytes + 1` through
`Medium.ReadStream` and closes the returned reader:

```go
reader, err := mount.Medium.ReadStream(relativePath)
if err != nil {
	return core.Fail(providerFailure("Preview", mount.ID, relativePath, err))
}
payload, err := goio.ReadAll(goio.LimitReader(reader, service.limits.MaxPreviewBytes+1))
closeErr := reader.Close()
if err != nil || closeErr != nil {
	if err == nil {
		err = closeErr
	}
	return core.Fail(providerFailure("Preview", mount.ID, relativePath, err))
}
truncated := int64(len(payload)) > service.limits.MaxPreviewBytes
if truncated {
	payload = payload[:service.limits.MaxPreviewBytes]
}
binary := !utf8.Valid(payload) || bytes.IndexByte(payload, 0) >= 0
```

For binary content, return no `Content`. Detect a small stable MIME set from
the extension and otherwise use `text/plain` or
`application/octet-stream`. Count lines only for text. Return mount ID and
relative path, never a host path.

- [ ] **Step 6: Make Wails methods thin and test the wire boundary**

`wails.go` exposes only:

```go
func (service *Service) ServiceName() string { return "Files" }
func (service *Service) ListMounts() core.Result
func (service *Service) ListDirectory(input ListDirectoryInput) core.Result
func (service *Service) Preview(input PreviewInput) core.Result
```

The receiver methods call internal helpers and return their existing
`core.Result`; they do not resolve home directories or paths.

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(ListDirectory|Preview)|TestServiceName' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the read façade**

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/listing.go \
  go/pkg/office/files/listing_test.go \
  go/pkg/office/files/preview.go \
  go/pkg/office/files/preview_test.go \
  go/pkg/office/files/wails.go \
  go/pkg/office/files/wails_test.go
git commit -m "feat(files): browse and preview Medium mounts"
```

---

### Task 3: Runtime metadata through a dedicated Medium

**Files:**

- Create: `go/pkg/office/files/runtime.go`
- Create: `go/pkg/office/files/runtime_test.go`
- Modify: `go/pkg/office/files/types.go`
- Modify: `go/pkg/office/files/listing.go`
- Modify: `go/pkg/office/files/preview.go`

**Interfaces:**

- Consumes: a distinct runtime `coreio.Medium`; it never reuses a content
  mount implicitly.
- Produces:
  `NewMediumRuntimeMetadata(coreio.Medium, string) RuntimeMetadata`,
  `NewMemoryRuntimeMetadata() RuntimeMetadata`, and the Medium-backed
  implementation of Task 1's runtime contract.

- [ ] **Step 1: Write failing persistence and recent-file tests**

```go
func TestMediumRuntimeMetadata_RoundTripGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(medium, "desktop/files/runtime.json")
	snapshot := RuntimeSnapshot{
		Version: 1,
		Favourites: []Favourite{{MountID: "documents", Path: ""}},
		Recent: []Recent{{
			MountID: "documents", Path: "notes/readme.md",
			Name: "readme.md", OpenedAt: "2026-07-26T12:00:00Z",
		}},
		Trash: []TrashReceipt{},
	}

	core.RequireNoError(t, runtime.Save(snapshot))
	got, err := runtime.Load()

	core.AssertNoError(t, err)
	core.AssertEqual(t, snapshot, got)
	core.AssertTrue(t, medium.IsFile("desktop/files/runtime.json"))
}

func TestMediumRuntimeMetadata_BadUnavailableDoesNotReset(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("desktop/files/runtime.json", `{"version":1}`))
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, readErr: fs.ErrPermission},
		"desktop/files/runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertTrue(t, base.IsFile("desktop/files/runtime.json"))
}

func TestService_PreviewRecordsRecentGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{memoryMount("documents", medium)}, runtime)

	result := service.Preview(PreviewInput{MountID: "documents", Path: "readme.md"})
	snapshot, err := runtime.Load()

	core.RequireTrue(t, result.OK)
	core.AssertNoError(t, err)
	core.AssertEqual(t, "readme.md", snapshot.Recent[0].Path)
}
```

- [ ] **Step 2: Run the runtime tests and observe missing types**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestMediumRuntimeMetadata|TestService_PreviewRecordsRecent' -count=1
```

Expected: FAIL because the Medium-backed runtime constructors do not exist.

- [ ] **Step 3: Implement the existing persistence interface**

```go
type mediumRuntimeMetadata struct {
	mu     sync.Mutex
	medium coreio.Medium
	path   string
}
```

`NewMemoryRuntimeMetadata` must call
`NewMediumRuntimeMetadata(coreio.NewMemoryMedium(), "runtime.json")`; even
ephemeral tests cross the same Medium boundary.

- [ ] **Step 4: Implement Medium-only load and staged save**

`Load` calls `Medium.Read`. Only `fs.ErrNotExist` returns
`RuntimeSnapshot{Version: 1, Favourites: []Favourite{},
Recent: []Recent{}, Trash: []TrashReceipt{}}`. Permission, transport, JSON, or
version errors are returned and never converted into an empty document.

`Save` serialises with `core.JSONMarshalString`, ensures the configured parent
and `.lthn-files/staging/runtime` through `Medium.EnsureDir`, then:

1. writes mode `0600` to a new operation staging file;
2. uses `Stat` to determine whether the configured document exists;
3. if present, renames it to an operation-specific backup in the owned staging
   directory;
4. renames the new staging file to the configured relative path;
5. on commit failure, restores the backup and deletes the new staging file; and
6. on success, deletes the backup through the same Medium.

`Load` first reads the configured document. If it is missing, it inspects only
the owned runtime-staging directory, validates candidate backup JSON, restores
the newest valid backup, and then loads it. Permission, transport, malformed
JSON, or failed recovery returns an error; it never becomes an empty document.
Tests inject failure at each rename and prove the previous valid document is
preserved or the recovery error remains explicit.

Guard `Load` and `Save` with one `sync.Mutex`.
`validateAndNormaliseRuntimeSnapshot` returns `(RuntimeSnapshot, error)` and is
applied both after decode and before save:

- `Version` is exactly `1`;
- every mount/path pair passes Task 1 validation;
- recents are de-duplicated newest-first and capped at 100;
- trash receipt paths may enter the internal namespace only inside this
  trusted runtime implementation; and
- all slices are non-nil.

Do not instantiate `go-store` here. The pinned go-store can use a Medium for
database transport, but it also stages SQLite/workspace files internally; it
is not the first implementation of this security boundary.

- [ ] **Step 5: Integrate metadata with catalogue and preview**

`ListMounts` loads runtime metadata and returns public mount DTOs plus only
favourites/recents whose mount still exists and whose path remains valid.
`Preview` records a successful text or binary preview after the stream closes.
A metadata-save failure returns a provider-unavailable failure rather than
pretending the read was fully recorded.

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestMediumRuntimeMetadata|TestService_(PreviewRecordsRecent|ListMounts)' \
  -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit runtime metadata**

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/runtime.go \
  go/pkg/office/files/runtime_test.go \
  go/pkg/office/files/listing.go \
  go/pkg/office/files/preview.go
git commit -m "feat(files): persist runtime metadata through Medium"
```

---

### Task 4: Capability-checked create, rename, and typed mutation events

**Files:**

- Create: `go/pkg/office/files/mutations.go`
- Create: `go/pkg/office/files/mutations_test.go`
- Rewrite: `go/pkg/office/files/events.go`
- Modify: `go/pkg/office/files/types.go`
- Modify: `go/pkg/office/files/wails.go`
- Modify: `go/pkg/office/files/wails_test.go`

**Interfaces:**

- Consumes: Task 1's per-mount locks, `Stat`, `EnsureDir`, and `Rename`.
- Produces:
  `CreateDirectory(CreateDirectoryInput) core.Result`,
  `Rename(RenameInput) core.Result`,
  `FileOperationResult`, `FileConflict`, `FileEvent`, and
  `Subscribe(*core.Core, func(*core.Core, FileEvent))`.

- [ ] **Step 1: Write failing mutation and event tests**

```go
func TestService_CreateDirectory_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("notes"))
	service := registeredMemoryService(t, "documents", medium, ReadWriteCapabilities())

	result := service.CreateDirectory(CreateDirectoryInput{
		MountID: "documents", ParentPath: "notes", Name: "Ideas",
	})

	core.RequireTrue(t, result.OK)
	_, err := medium.Stat("notes/Ideas")
	core.AssertNoError(t, err)
}

func TestService_CreateDirectory_BadConflict(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("Ideas", "already a file"))
	service := registeredMemoryService(t, "documents", medium, ReadWriteCapabilities())

	result := service.CreateDirectory(CreateDirectoryInput{
		MountID: "documents", Name: "Ideas",
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationConflict, operation.Status)
	core.AssertEqual(t, ErrorConflict, operation.Code)
}

func TestService_Rename_UglyRejectsRootAndInternal(t *core.T) {
	service := registeredMemoryService(
		t, "documents", coreio.NewMemoryMedium(), ReadWriteCapabilities(),
	)
	for _, path := range []string{"", ".lthn-files"} {
		result := service.Rename(RenameInput{
			MountID: "documents", Path: path, Name: "moved",
		})
		core.AssertFalse(t, result.OK, path)
		core.AssertContains(t, result.Error(), string(ErrorBoundaryRejected))
	}
}

func TestService_Rename_EmitsRelativeEventGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("draft.txt", "hello"))
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	service := NewService(Options{
		Mounts: []Mount{memoryMount("documents", medium, ReadWriteCapabilities())},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	var received FileEvent
	Subscribe(c, func(_ *core.Core, event FileEvent) { received = event })

	result := service.Rename(RenameInput{
		MountID: "documents", Path: "draft.txt", Name: "final.txt",
	})

	core.RequireTrue(t, result.OK)
	core.AssertEqual(t, "rename", received.Operation)
	core.AssertElementsMatch(t, []string{"draft.txt", "final.txt"}, received.Paths)
	core.AssertNotContains(t, core.JSONMarshalString(received), "/Users/")
}
```

- [ ] **Step 2: Run the new tests and observe the missing mutation API**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(CreateDirectory|Rename)' -count=1
```

Expected: FAIL because the input, result, event, and methods do not exist.

- [ ] **Step 3: Add stable operation and event wire shapes**

```go
type OperationStatus string

const (
	OperationCompleted OperationStatus = "completed"
	OperationConflict  OperationStatus = "conflict"
	OperationPartial   OperationStatus = "partial"
)

type FileAddress struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

type FileConflict struct {
	Source      FileAddress `json:"source"`
	Destination FileAddress `json:"destination"`
	Kind        EntryKind   `json:"kind"`
}

type FileOperationResult struct {
	OperationID string          `json:"operationId"`
	Operation   string          `json:"operation"`
	Status      OperationStatus `json:"status"`
	Code        ErrorCode       `json:"code,omitempty"`
	Source      FileAddress     `json:"source"`
	Destination *FileAddress    `json:"destination,omitempty"`
	Affected    []FileAddress   `json:"affected"`
	Conflict    *FileConflict   `json:"conflict,omitempty"`
	Message     string          `json:"message"`
}

type CreateDirectoryInput struct {
	MountID   string `json:"mountId"`
	ParentPath string `json:"parentPath,omitempty"`
	Name      string `json:"name"`
}

type RenameInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
	Name    string `json:"name"`
}

type FileEvent struct {
	Operation   string        `json:"operation"`
	OperationID string        `json:"operationId"`
	MountIDs    []string      `json:"mountIds"`
	Paths       []string      `json:"paths"`
	At          core.Time     `json:"at"`
}
```

Keep `FileEvent` deliberately smaller than `FileOperationResult`; it is an
invalidation signal, not a second state store. `events.go` mirrors the
repository's typed ACTION pattern:

```go
func Subscribe(c *core.Core, fn func(*core.Core, FileEvent)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, message core.Message) core.Result {
		if event, ok := message.(FileEvent); ok {
			fn(c, event)
		}
		return core.Ok(nil)
	})
}
```

- [ ] **Step 4: Implement create and rename through one Medium**

Both methods:

1. resolve the mount;
2. require the matching declared capability;
3. validate the parent/path and single-entry name;
4. acquire that mount's mutex;
5. use `Stat` to verify parents and detect conflicts;
6. call only `EnsureDir` or `Rename`;
7. build an operation result containing relative addresses; and
8. call `service.fireEvent` only after success.

The empty parent denotes the already-registered mount root. Do not call
`Stat("")` merely to rediscover that root: Memory Medium represents it
implicitly, while local `os.Root` represents it as `"."`. Non-empty parents
must be checked with `Stat`.

A destination conflict is a successfully transported typed outcome with
`Status: OperationConflict`, `Code: ErrorConflict`, and a `FileConflict`
containing the source and destination addresses. This preserves both addresses
for Angular. It performs no mutation and emits no event. Invalid input,
capability denial, boundary rejection, and provider failure remain
`core.Fail`.

Map only `fs.ErrNotExist` to `files.missing_entry` and `fs.ErrPermission` to
`files.capability_denied`. All other provider errors become
`files.provider_unavailable` with a calm renderer message; log the operation,
mount ID, relative path, and wrapped cause on the Go side without serialising
the cause.

`Rename` takes a replacement single name, derives the destination under the
same parent, rejects the mount root, and never overwrites. It must not accept a
destination path from the renderer.

- [ ] **Step 5: Expose thin Wails methods and run the mutation suite**

Add the two receiver methods to `wails.go` and verify their JSON-visible inputs
contain no absolute-root field.

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(CreateDirectory|Rename)|TestEvents_' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the first mutations**

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/mutations.go \
  go/pkg/office/files/mutations_test.go \
  go/pkg/office/files/events.go \
  go/pkg/office/files/wails.go \
  go/pkg/office/files/wails_test.go
git commit -m "feat(files): add Medium mutations and events"
```

---

### Task 5: Bounded streaming copy and same/cross-Medium move

**Files:**

- Create: `go/pkg/office/files/transfer.go`
- Create: `go/pkg/office/files/transfer_test.go`
- Modify: `go/pkg/office/files/service.go`
- Modify: `go/pkg/office/files/listing.go`
- Modify: `go/pkg/office/files/types.go`
- Modify: `go/pkg/office/files/wails.go`

**Interfaces:**

- Consumes: source and destination registered media, Task 1 limits, and Task 4
  operation results/events.
- Produces:
  `Copy(TransferInput) core.Result`,
  `Move(TransferInput) core.Result`, deterministic preflight, staging, and
  partial-move reporting.

- [ ] **Step 1: Write failing cross-Medium and adversarial tests**

```go
func TestService_CopyAcrossMedia_Good(t *core.T) {
	source := coreio.NewMemoryMedium()
	destination := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("project/readme.md", "hello"))
	core.RequireNoError(t, source.Write("project/src/main.go", "package main"))
	service := registeredService(t, []Mount{
		memoryMount("projects", source, ReadWriteCapabilities()),
		memoryMount("backup", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "projects", Path: "project"},
		Destination: FileAddress{MountID: "backup", Path: "archive/project"},
	})

	core.RequireTrue(t, result.OK)
	content, err := destination.Read("archive/project/src/main.go")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "package main", content)
}

func TestService_Copy_UglyRejectsLink(t *core.T) {
	source := &entryMedium{
		Medium: coreio.NewMemoryMedium(),
		entries: map[string][]fs.DirEntry{
			"": {coreio.NewDirEntry(
				"escape", false, fs.ModeSymlink,
				coreio.NewFileInfo("escape", 0, fs.ModeSymlink, core.Now(), false),
			)},
		},
	}
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", coreio.NewMemoryMedium(), ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "escape"},
		Destination: FileAddress{MountID: "destination", Path: "escape"},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorUnsupportedEntry))
}

func TestService_MoveAcrossMedia_BadSourceDeleteIsPartial(t *core.T) {
	source := &failingMedium{
		Medium: coreio.NewMemoryMedium(), deleteErr: fs.ErrPermission,
	}
	core.RequireNoError(t, source.Write("report.md", "done"))
	destination := coreio.NewMemoryMedium()
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Move(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{MountID: "destination", Path: "report.md"},
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationPartial, operation.Status)
	core.AssertEqual(t, ErrorPartialMove, operation.Code)
	content, err := destination.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "done", content)
}
```

Add limit tests for depth, item count, bytes, a provider growing a file after
preflight, a mid-stream write failure, same-source/destination rejection, and a
destination conflict. Extend `failingMedium` with stream/delete errors rather
than using raw files.

- [ ] **Step 2: Run transfer tests and observe the missing implementation**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(Copy|Move)' -count=1
```

Expected: FAIL because `TransferInput`, `Copy`, and `Move` do not exist.

- [ ] **Step 3: Define the transfer input and internal preflight manifest**

```go
type TransferInput struct {
	Source      FileAddress `json:"source"`
	Destination FileAddress `json:"destination"`
}

type transferEntry struct {
	SourcePath      string
	DestinationPath string
	Kind            EntryKind
	Mode            fs.FileMode
	SizeBytes       int64
	Depth           int
}
```

There is intentionally no `Overwrite` boolean. A present destination returns
an `OperationConflict` result with code `files.conflict`, and the UI can ask
for another name; destructive replacement is not smuggled into copy/move.

- [ ] **Step 4: Implement fail-closed preflight**

`preflightTransfer` recursively inspects only through the source Medium:

- call `Stat` on the root and `List` on directories;
- check `DirEntry.Type()&fs.ModeSymlink` before `Info`;
- reject links and non-regular/non-directory entries;
- sort each directory before traversal;
- count every entry, depth, and total regular-file bytes;
- enforce `MaxRecursiveDepth`, `MaxRecursiveItems`, and
  `MaxTransferBytes` while walking; and
- reject source root, destination root, internal namespace, and a destination
  nested under the source on the same mount.

Use `Stat` on the destination. Only `fs.ErrNotExist` means it is available.
The preflight manifest stores provider-relative paths and modes only.

- [ ] **Step 5: Stream into an owned staging directory**

During service registration, initialise and verify this exact marker through
each writable destination Medium:

```text
.lthn-files/owner.json
{"owner":"ai.lthn.desktop.files","version":1}
```

If `.lthn-files` exists without that exact marker, do not alter it and disable
staging/trash capabilities for the mount. If it is absent, create the
directory and marker through the Medium. Ordinary listing always hides the
namespace.

Track `internalReady` per mount under the service lock. `ListMounts` returns
effective capabilities: a mount whose namespace is unowned has `CopyTo`,
`Trash`, and `Restore` false. `Copy`, cross-Medium `Move`, `Trash`, and
`Restore` recheck readiness under the mount lock rather than trusting the
renderer-visible flag. Create Directory, Rename, read, and same-mount Move
remain usable when their own capabilities permit them.

Copy into `.lthn-files/staging/<operation-id>/payload`. Create directories with
`EnsureDir`; for each file:

1. open `ReadStream` on the source;
2. open `WriteStream` on the staging destination;
3. copy with a 128 KiB buffer;
4. maintain actual byte and entry counters during the copy;
5. close the writer and reader, treating either close error as failure; and
6. abort when actual bytes exceed the preflight/server limit.

On failure, delete only the owned operation staging path through
`Medium.DeleteAll` and return the affected relative addresses. On success,
rename the staging payload to the destination and remove the empty operation
directory. Never stage under `/tmp` or another host path.

- [ ] **Step 6: Implement move semantics**

- Same mount: after all validation and conflict checks, call
  `Medium.Rename(source, destination)` while holding its one lock.
- Different mounts: perform the staged copy, then delete the source with
  `Delete` for a file or `DeleteAll` for a directory.
- If destination commit succeeds and source deletion fails, return
  an OK `FileOperationResult` with `Status: OperationPartial`,
  `Code: ErrorPartialMove`, and both source and destination addresses. Do not
  delete the destination or describe the move as atomic.
- Acquire two mount locks in lexical mount-ID order and release in reverse
  order to prevent deadlock.

Emit exactly one event after a complete copy/move, and one `partial` event for
a partial move.

- [ ] **Step 7: Run focused tests and commit**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(Copy|Move)|TestTransfer_' -count=1
```

Expected: PASS.

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/service.go \
  go/pkg/office/files/listing.go \
  go/pkg/office/files/transfer.go \
  go/pkg/office/files/transfer_test.go \
  go/pkg/office/files/wails.go
git commit -m "feat(files): stream bounded Medium transfers"
```

---

### Task 6: Medium-owned trash, restore, and confirmed permanent deletion

**Files:**

- Create: `go/pkg/office/files/trash.go`
- Create: `go/pkg/office/files/trash_test.go`
- Modify: `go/pkg/office/files/runtime.go`
- Modify: `go/pkg/office/files/types.go`
- Modify: `go/pkg/office/files/wails.go`

**Interfaces:**

- Consumes: Task 3 runtime receipts and Task 5's verified internal namespace.
- Produces:
  `Trash(TrashInput) core.Result`,
  `ListTrash() core.Result`,
  `Restore(RestoreInput) core.Result`, and
  `Delete(DeleteInput) core.Result`.

- [ ] **Step 1: Write failing trash lifecycle tests**

```go
func TestService_TrashRestore_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("notes/idea.md", "ship it"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	trashed := service.Trash(TrashInput{
		MountID: "documents", Path: "notes/idea.md",
	})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	listed := service.ListTrash()
	core.RequireTrue(t, listed.OK)
	core.AssertEqual(t, receiptID, listed.Value.(TrashSnapshot).Entries[0].ReceiptID)

	restored := service.Restore(RestoreInput{ReceiptID: receiptID})
	core.RequireTrue(t, restored.OK)
	content, err := medium.Read("notes/idea.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "ship it", content)
}

func TestService_Trash_BadUnownedNamespace(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(".lthn-files/unrelated", "foreign data"))
	core.RequireNoError(t, medium.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	result := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
	content, err := medium.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "content", content)
}

func TestService_Delete_UglyRequiresRecursiveConfirmation(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("archive/report.md", "content"))
	service := registeredMemoryService(
		t, "documents", medium, ReadWriteCapabilities(),
	)

	for _, input := range []DeleteInput{
		{MountID: "documents", Path: "archive"},
		{MountID: "documents", Path: "archive", Recursive: true},
	} {
		result := service.Delete(input)
		core.AssertFalse(t, result.OK)
		core.AssertContains(t, result.Error(), string(ErrorInvalidInput))
	}
}
```

Also cover restore conflict, stale receipt, runtime-save failure with successful
rollback, rollback failure as partial, permanent deletion of a trash receipt,
root deletion rejection, missing-provider errors, and internal-path rejection.

- [ ] **Step 2: Run the lifecycle tests and observe missing methods**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(Trash|ListTrash|Restore|Delete)' -count=1
```

Expected: FAIL because the trash DTOs and lifecycle methods do not exist.

- [ ] **Step 3: Define the public trash contract**

```go
type TrashInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

type RestoreInput struct {
	ReceiptID string `json:"receiptId"`
}

type DeleteInput struct {
	MountID   string `json:"mountId,omitempty"`
	Path      string `json:"path,omitempty"`
	ReceiptID string `json:"receiptId,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
	Confirmed bool   `json:"confirmed"`
}

type TrashEntry struct {
	ReceiptID   string    `json:"receiptId"`
	MountID     string    `json:"mountId"`
	OriginalPath string   `json:"originalPath"`
	Name        string    `json:"name"`
	Kind        EntryKind `json:"kind"`
	SizeBytes   int64     `json:"sizeBytes"`
	TrashedAt   string    `json:"trashedAt"`
	Available   bool      `json:"available"`
	ErrorCode   ErrorCode `json:"errorCode,omitempty"`
}

type TrashSnapshot struct {
	Entries []TrashEntry `json:"entries"`
	RefreshedAt string   `json:"refreshedAt"`
}
```

Add `ReceiptID string` to `FileOperationResult`. `DeleteInput` requires exactly
one of `{MountID, Path}` or `ReceiptID`.

- [ ] **Step 4: Implement trash and restore transaction order**

Trash:

1. validate the source, capability, non-root rule, and owned namespace;
2. `Stat` the source;
3. rename it to
   `.lthn-files/trash/<core.ID()>/payload` through the same Medium;
4. append a runtime receipt containing only mount ID and relative paths; and
5. save the runtime document.

If step 5 fails, rename the payload back. A failed rollback returns
an `OperationPartial` result with code `files.partial_move`, preserves the
internal payload address only in Go logs, and returns the receipt ID so
recovery remains possible.

Restore reverses that order: load and validate the receipt, reject a
destination conflict with an `OperationConflict` outcome, rename the payload
to its original path, remove the receipt, then save. If save fails, rename the
entry back into trash; a failed rollback is partial and never reported as
restored.

`ListTrash` loads receipts and calls `Stat` through each receipt's registered
Medium. `fs.ErrNotExist` marks the row stale and returns a typed unavailable
row; other provider errors fail the snapshot. It never scans arbitrary
`.lthn-files` children.

- [ ] **Step 5: Implement explicit permanent deletion**

- `Confirmed` is mandatory for every permanent delete.
- A non-empty directory additionally requires `Recursive`.
- Direct mount/path deletion requires `Capabilities.Delete`, validates the
  client path, and rejects the root/internal namespace.
- Receipt deletion resolves the trusted internal path from runtime state,
  requires the mount's delete capability, removes with `Delete` or `DeleteAll`,
  and removes the receipt only after Medium success.
- Use `List` to decide whether a directory is non-empty; do not use a raw
  directory API or convenience boolean.

Emit relative-path events for trash, restore, and delete. The trash event may
include the receipt ID but never the trusted internal path.

- [ ] **Step 6: Run the lifecycle suite and commit**

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestService_(Trash|ListTrash|Restore|Delete)|TestTrash_' -count=1
```

Expected: PASS.

```bash
git add go/pkg/office/files/types.go \
  go/pkg/office/files/runtime.go \
  go/pkg/office/files/trash.go \
  go/pkg/office/files/trash_test.go \
  go/pkg/office/files/wails.go
git commit -m "feat(files): add Medium-owned trash lifecycle"
```

---

### Task 7: Prove the upstream local-Medium race and lifecycle contract

**Repository:** `/Users/snider/Code/core/go-io`

**Files:**

- Modify: `go/local/medium.go` — add a nil-by-default scheduling seam after
  current validation so the existing race fails behaviourally.
- Create: `go/local/medium_root_test.go`
- Modify: `go/local/medium_test.go`
- Modify: `go/service_test.go`

**Interfaces:**

- Tests the unchanged public `local.New(string) (*Medium, error)` and
  `coreio.NewSandboxed(string) (coreio.Medium, error)` contracts.
- Establishes the release gate for Go 1.26 `os.Root` containment and closure.

- [ ] **Step 1: Preserve the upstream worktree before editing**

Run:

```bash
cd /Users/snider/Code/core/go-io
git status --short
git branch --show-current
git log -1 --oneline
```

Expected baseline: branch `dev`, current commit recorded in the execution
notes, and the existing untracked `go.work.sum` left untouched. Do not clean,
stage, or rewrite unrelated upstream work.

- [ ] **Step 2: Write a deterministic component-swap test**

Add an unexported nil-by-default `beforeRootOperation func()` field to the
current `Medium`, and invoke it after `validatePath` but immediately before
each unrestricted filesystem call. This is only a deterministic scheduler for
the already-existing validation/use gap. The test sets it to atomically
replace an in-root directory with a symlink to an outside directory:

```go
func TestLocal_Read_ComponentSwapCannotEscape_Ugly(t *core.T) {
	root := t.TempDir()
	outside := t.TempDir()
	core.RequireNoError(t, os.WriteFile(
		core.Path(outside, "secret.txt"), []byte("outside"), 0600,
	))
	core.RequireNoError(t, os.MkdirAll(core.Path(root, "pivot"), 0700))
	core.RequireNoError(t, os.WriteFile(
		core.Path(root, "pivot", "secret.txt"), []byte("inside"), 0600,
	))
	medium, err := New(root)
	core.RequireNoError(t, err)
	medium.beforeRootOperation = func() {
		core.RequireNoError(t, os.Rename(
			core.Path(root, "pivot"), core.Path(root, "held"),
		))
		core.RequireNoError(t, syscall.Symlink(outside, core.Path(root, "pivot")))
	}

	content, err := medium.Read("pivot/secret.txt")

	core.AssertError(t, err)
	core.AssertNotEqual(t, "outside", content)
}
```

The upstream provider test is allowed to use OS/Core primitives to construct
an attack outside the provider boundary. Product Files tests are not.

- [ ] **Step 3: Run the red security tests against v0.15.1 behaviour**

Run:

```bash
cd /Users/snider/Code/core/go-io/go
go test ./local -run \
  'TestLocal_Read_ComponentSwapCannotEscape_Ugly' -count=1
```

Expected: FAIL because the scheduled read returns `"outside"` through the
validate-then-use gap.

- [ ] **Step 4: Add the rest of the root/lifecycle contract**

After recording the behavioural red result, add sibling tests for:

- an internal relative symlink which remains inside the root and is readable;
- an absolute or escaping symlink which is rejected;
- `Open`, `Create`, `Append`, `List`, `Stat`, `Delete`, `DeleteAll`, and
  `Rename` under the same scheduled swap;
- root deletion refusal;
- protected HOME refusal for the global `/` root;
- use-after-`Close` returning errors; and
- service shutdown closing a configured sandboxed Medium.

Do not commit the deliberately failing exploit state. Keep the red tests and
nil-by-default seam in the worktree and proceed directly to Task 8; the first
upstream commit contains tests and the repair together, with the red command
preserved in the plan execution notes.

---

### Task 8: Replace upstream local path re-resolution with `os.Root`

**Repository:** `/Users/snider/Code/core/go-io`

**Files:**

- Rewrite: `go/local/medium.go`
- Delete: `go/local/medium_link.go`
- Delete: `go/local/medium_link_test.go`
- Modify: `go/local/medium_test.go`
- Create: `go/local/medium_root_test.go`
- Modify: `go/service.go`
- Modify: `go/service_test.go`

**Interfaces:**

- Preserves the public `io.Medium` interface.
- Adds `(*local.Medium).Close() error` as an optional `io.Closer`.
- Uses Go 1.26 `*os.Root` for every local provider operation.

- [ ] **Step 1: Replace the Medium internals**

Use this structural shape:

```go
type Medium struct {
	filesystemRoot      string
	root                *os.Root
	closeOnce           sync.Once
	closeErr            error
	beforeRootOperation func() // unexported deterministic security-test seam
}

func New(rootPath string) (*Medium, error) {
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return nil, core.E("local.New", "os.Root containment unavailable", fs.ErrPermission)
	}
	absoluteRoot := absolutePath(rootPath)
	root, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return nil, core.E("local.New", "open rooted filesystem", err)
	}
	return &Medium{filesystemRoot: root.Name(), root: root}, nil
}
```

`beforeRootOperation` is nil in production and invoked immediately before each
`os.Root` call. It must never receive or expose a path and must not be exported.

Replace `validatePath` with a pure `rootPath` normaliser which:

- maps the Medium root to `"."`;
- rejects NUL and names which `os.Root` cannot represent;
- preserves the existing sandbox normalisation by cleaning against an
  artificial leading separator, so `/etc/passwd`, `../file`, and
  `dir/../file` remain root-relative rather than escaping or becoming a new
  host-path interpretation;
- converts accepted legacy separators before passing a relative name to
  `os.Root`;
- preserves the global Medium's legacy relative-to-current-working-directory
  behaviour by converting that trusted CWD to a root-relative name; and
- returns a relative slash/path accepted by `os.Root`.

Delete `unrestrictedFileSystem`, symlink-resolution helpers, and every
validate-then-use call.

- [ ] **Step 2: Map every Medium operation to the rooted handle**

Implement the complete interface with these primitives:

| Medium method | Root operation |
|---|---|
| `Read` | `root.ReadFile` |
| `Write` / `WriteMode` | `root.MkdirAll(parent)` then `root.WriteFile` |
| `EnsureDir` | `root.MkdirAll` |
| `List` | `fs.ReadDir(root.FS(), name)` |
| `Stat` | `root.Stat` |
| `Open` / `ReadStream` | `root.Open` |
| `Create` / `WriteStream` | `root.MkdirAll(parent)` then `root.OpenFile(O_CREATE|O_TRUNC|O_WRONLY)` |
| `Append` | `root.MkdirAll(parent)` then `root.OpenFile(O_CREATE|O_APPEND|O_WRONLY)` |
| `Delete` | `root.Remove` |
| `DeleteAll` | `root.RemoveAll` |
| `Rename` | `root.Rename` |
| `Exists` / `IsFile` / `IsDir` | `root.Stat`, retaining the legacy boolean surface |

Keep root/protected-path deletion checks before `Remove`/`RemoveAll`, but make
the operation itself handle-relative. Do not retain a path-string fallback on
any error.

`Close` calls `root.Close()` once. All methods reject a nil/closed root rather
than falling back. `Service.OnShutdown` checks whether its configured Medium
implements `io.Closer`; it closes only service-owned sandboxed media, not the
package-global `io.Local`.

- [ ] **Step 3: Run focused and complete upstream verification**

Run:

```bash
cd /Users/snider/Code/core/go-io/go
gofmt -w local/medium.go local/medium_test.go local/medium_root_test.go \
  service.go service_test.go
go test ./local -count=1
go test ./... -count=1
go vet ./...
```

Expected: all PASS. Confirm the deleted helper has no references:

```bash
rg -n 'validatePath|resolveSymlinks|unrestrictedFileSystem' local
```

Expected: no matches.

- [ ] **Step 4: Commit the upstream security repair**

```bash
cd /Users/snider/Code/core/go-io
git diff --check
git add go/local/medium.go \
  go/local/medium_test.go \
  go/local/medium_root_test.go \
  go/local/medium_link.go \
  go/local/medium_link_test.go \
  go/service.go \
  go/service_test.go
git commit -m "fix(local): anchor sandbox operations to os.Root"
```

Do not stage the pre-existing `go.work.sum`.

- [ ] **Step 5: Release checkpoint**

Use the repository's existing release process to publish `go/v0.15.2` from the
verified commit. Creating or pushing the release/tag is an external mutation,
so execution pauses here unless the owner has explicitly authorised that
release action. Record the immutable commit and tag before Desktop changes its
module version; do not use a `replace` directive or a local checkout as the
product dependency.

---

### Task 9: Pin the repaired provider and compose the sole Files service

**Repository:** Lethean Desktop

**Files:**

- Modify: `go/go.mod`
- Modify: `go/go.sum`
- Modify: `go/pkg/office/files/default_mounts.go`
- Modify: `go/pkg/office/files/service.go`
- Modify: `go/pkg/office/files/types.go`
- Create: `go/pkg/office/files/medium_boundary_test.go`
- Rewrite: `go/pkg/office/files/files_example_test.go`
- Delete: `go/pkg/office/files/diskusage_unix.go`
- Delete: `go/pkg/office/files/diskusage_windows.go`
- Delete: `go/pkg/files/files.go`
- Delete: `go/pkg/files/files_test.go`
- Modify: `go/cmd/lthn/app.go`
- Modify: `go/pkg/desktop/desktop.go`
- Create: `go/pkg/desktop/files_events.go`
- Create: `go/pkg/desktop/files_events_test.go`
- Modify: `go/pkg/terminal/service.go`

**Interfaces:**

- Pins the released `dappco.re/go/io v0.15.2`.
- Composes app-state metadata on the registered `"io"` service Medium.
- Composes narrow local content mounts with `coreio.NewSandboxed`.
- Binds exactly one Wails service named `Files`.

- [ ] **Step 1: Verify and pin the immutable upstream release**

Run:

```bash
cd /Users/snider/Lethean/agent/cladius/Code/lthn/desktop/go
go list -m -json dappco.re/go/io@v0.15.2
go get dappco.re/go/io@v0.15.2
go mod tidy
go list -m dappco.re/go/io
```

Expected final output:

```text
dappco.re/go/io v0.15.2
```

Inspect `go.work.sum` before staging because it was already modified at plan
time. Preserve unrelated content; stage it only if the release resolution
adds a directly relevant checksum hunk.

- [ ] **Step 2: Write failing default-composition and shutdown tests**

Use only media to seed the temporary home:

```go
func TestDefaultOptions_LocalMountsUseSandboxedMedia_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	homeMedium, err := coreio.NewSandboxed(home)
	core.RequireNoError(t, err)
	core.RequireNoError(t, homeMedium.EnsureDir("Documents"))
	core.RequireNoError(t, homeMedium.EnsureDir("Downloads"))
	core.RequireNoError(t, homeMedium.EnsureDir("Code"))
	core.RequireNoError(t, homeMedium.EnsureDir("Lethean/conf/models"))
	t.Cleanup(func() {
		if closer, ok := homeMedium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	dataRoot := t.TempDir()
	c := core.New(core.WithName("io", coreio.NewService(coreio.IOConfig{Root: dataRoot})))

	result := DefaultOptions(c)

	core.RequireTrue(t, result.OK)
	options := result.Value.(Options)
	core.AssertElementsMatch(
		t,
		[]string{"documents", "downloads", "models", "projects"},
		mountIDs(options.Mounts),
	)
	core.AssertNotNil(t, options.Runtime)
}

func TestService_OnShutdownClosesOwnedMounts_Ugly(t *core.T) {
	medium := &closingMedium{Medium: coreio.NewMemoryMedium()}
	service := NewService(Options{
		Mounts: []Mount{{
			ID: "documents", Name: "Documents", Medium: medium, Owned: true,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)

	result := service.OnShutdown(core.Background())

	core.AssertTrue(t, result.OK)
	core.AssertEqual(t, 1, medium.closeCalls)
}
```

Add a Bad test proving a configured mount path which does not exist is skipped
only for `fs.ErrNotExist`; permission or rooted-provider creation errors fail
composition. Add a test proving runtime metadata is written at
`desktop/files/runtime.json` through the registered I/O Medium.

- [ ] **Step 3: Compose runtime and narrow local mounts**

`DefaultOptions(c)` must:

1. resolve `*coreio.Service` from the Core registry name `"io"` and fail if it
   or its Medium is unavailable;
2. create runtime metadata with
   `NewMediumRuntimeMetadata(ioService.Medium,
   "desktop/files/runtime.json")`;
3. obtain HOME from `core.UserHomeDir` only as trusted mount configuration;
4. derive the following roots lexically, without scanning or reading them:

   | Mount ID | Root |
   |---|---|
   | `documents` | `$HOME/Documents` |
   | `downloads` | `$HOME/Downloads` |
   | `projects` | `$HOME/Code` |
   | `models` | `$HOME/Lethean/conf/models` |
   | `recordings` | `$HOME/Recordings` |
   | `screenshots` | `$HOME/Screenshots` |

5. call `coreio.NewSandboxed(root)` for each root;
6. skip a missing optional root, fail on every other error; and
7. set `Owned: true` and `ContainmentAudited: true` on every successfully
   opened content mount; the latter is valid only because Step 1 pinned the
   repaired rooted provider.

Do not call `paths.ModelsDir()` here: its current override read and directory
creation use legacy Core filesystem wrappers. User-configured model roots can
become managed mounts after that configuration source itself moves behind a
Medium. Do not create an unrestricted home mount.

Add this field to the internal `Mount` shape:

```go
Owned bool `json:"-"`
```

It is lifecycle metadata, never a renderer capability.
`Service.OnShutdown` closes only `Mount.Owned` media implementing
`io.Closer`. It does not close the application `"io"` Medium which owns
runtime metadata.

- [ ] **Step 4: Add the non-negotiable Files source boundary test**

`medium_boundary_test.go` opens its own package directory through
`coreio.NewSandboxed(".")`, lists and reads source through that Medium, then
uses `go/parser` and `go/ast` to scan every non-test `.go` file. Close the
sandboxed Medium in test cleanup. Fail on:

- imports `os`, `path/filepath`, or `syscall`;
- selectors `core.ReadFile`, `core.WriteFile`, `core.ReadDir`, `core.DirFS`,
  `core.Stat`, `core.Lstat`, `core.Open`, `core.Create`, or
  `core.NewUnrestrictedFS`;
- a `core.Fs` composite/accessor; and
- any Wails input field named `Root`, `AbsolutePath`, `Endpoint`,
  `Credential`, `Secret`, or `Key`.

The test explicitly permits standard `io`, `io/fs`, and provider construction
in `default_mounts.go`. Parse `wails.go` separately: it must contain no
`.Medium` selector, and every public Files operation must be a one-return
delegation to its exact unexported implementation helper (`listMounts`,
`listDirectory`, `preview`, `createDirectory`, `rename`, `copy`, `move`,
`trash`, `listTrash`, `restore`, or `delete`). The helper tests prove mount
resolution and capability checks before provider calls.

Run:

```bash
go test ./go/pkg/office/files -run \
  'TestFilesMediumBoundary|TestDefaultOptions|TestService_OnShutdown' -count=1
```

Expected: PASS.

- [ ] **Step 5: Remove both bypass and duplicate native bindings**

- Delete `go/pkg/files`; its unrestricted absolute-path `Read` surface has no
  compatibility grace period.
- Delete disk-usage syscall files and the old local-location/recent DTOs.
- Remove `dappco.re/lthn/desktop/pkg/files` from
  `go/pkg/desktop/desktop.go`.
- Remove `gui.Bind(files.NewService(s.opts.Core))`.
- Keep only the Core-registered `office-files` lookup and
  `gui.Bind(filesSvc)`.
- Update terminal comments which used the retired package as a trust-model
  comparison.
- Keep `core.WithName("office-files", files.Register)` in `app.go`; update its
  comment to describe registered Medium mounts and ensure `"io"` remains
  registered earlier.

Add a source-contract test in `go/pkg/desktop` which parses the binding slice
and asserts one `Files` binding source.

- [ ] **Step 6: Relay typed Core events to Wails**

`files_events.go` subscribes once:

```go
func registerFilesEvents(c *core.Core) {
	officefiles.Subscribe(c, func(c *core.Core, event officefiles.FileEvent) {
		emitCoreEvent(c, "lthn:files:changed", event)
	})
}
```

Call it during desktop setup before the GUI starts. Test the exact event name
and payload with mount IDs/relative paths, and assert serialised output has no
temporary root. The relay must not perform a refresh or file read itself.

- [ ] **Step 7: Replace examples with Memory-Medium examples**

Provide runnable examples for `ListMounts`, `ListDirectory`, `Preview`, and
one mutation. They must seed with `coreio.NewMemoryMedium`, register the
service, and print deterministic provider-neutral output:

```go
func ExampleService_ListDirectory() {
	medium := coreio.NewMemoryMedium()
	_ = medium.Write("notes/readme.md", "hello")
	service := files.NewService(files.Options{
		Mounts: []files.Mount{{
			ID: "documents", Name: "Documents", Kind: "memory",
			Capabilities: files.ReadWriteCapabilities(), Medium: medium,
			ContainmentAudited: true,
		}},
		Runtime: files.NewMemoryRuntimeMetadata(),
	})
	_ = service.Register(core.New())
	result := service.ListDirectory(files.ListDirectoryInput{
		MountID: "documents",
	})
	snapshot := result.Value.(files.DirectorySnapshot)
	core.Println(snapshot.Entries[0].Name)
	// Output: notes
}
```

- [ ] **Step 8: Run the wired Go slice and commit**

Run:

```bash
go test ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn
```

Expected: PASS. If a running development app owns `127.0.0.1:9099`, close it
and rerun the one affected desktop test rather than changing the port contract.

```bash
git add go/go.mod go/go.sum \
  go/pkg/office/files \
  go/pkg/files \
  go/cmd/lthn/app.go \
  go/pkg/desktop/desktop.go \
  go/pkg/desktop/files_events.go \
  go/pkg/desktop/files_events_test.go \
  go/pkg/terminal/service.go
git commit -m "feat(files): bind the sole Medium-backed service"
```

---

### Task 10: Typed Files state, reversible navigation, and the full demo catalogue

**Files:**

- Create:
  `frontend-ng/src/app/desktop/apps/files/files-view.models.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-view-state.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-view-state.spec.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-demo.data.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-demo.store.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-demo.store.spec.ts`

**Interfaces:**

- Consumes: the Go camel-case JSON DTOs from Tasks 1–6.
- Produces: readonly renderer models, pure route-token functions, state
  reconciliation, and a per-window in-memory demo implementation.

- [ ] **Step 1: Write failing token, state, and demo-store tests**

```ts
describe('Files navigation tokens', () => {
  it('keeps existing root mount ids and reversibly encodes nested paths', () => {
    expect(filesToken({ kind: 'directory', mountId: 'documents', path: '' }))
      .toBe('documents');
    const token = filesToken({
      kind: 'directory',
      mountId: 'documents',
      path: 'Invoices/2026 July',
    });
    expect(parseFilesToken(token)).toEqual({
      kind: 'directory',
      mountId: 'documents',
      path: 'Invoices/2026 July',
    });
  });

  it.each(['/etc', '../secret', 'documents::%ZZ', 'documents::a%5Cb'])(
    'fails closed to home for %s',
    (token) => expect(parseFilesToken(token)).toEqual({ kind: 'home' }),
  );
});

describe('FilesDemoStore', () => {
  it('retains the complete nested design fixture', async () => {
    const store = new FilesDemoStore();

    const home = await store.listHome();
    const documents = await store.listDirectory({
      mountId: 'documents',
      path: '',
      cursor: '',
      limit: 200,
    });
    const invoices = await store.listDirectory({
      mountId: 'documents',
      path: 'Invoices',
      cursor: '',
      limit: 200,
    });

    expect(home.entries.map(({ name }) => name)).toContain('welcome.txt');
    expect(documents.entries.map(({ name }) => name)).toContain('whitepaper.pdf');
    expect(invoices.entries).toEqual([]);
  });

  it('mutates only its own in-memory catalogue', async () => {
    const first = new FilesDemoStore();
    const second = new FilesDemoStore();

    await first.createDirectory({
      mountId: 'documents',
      parentPath: '',
      name: 'Ideas',
    });

    expect((await first.listDirectory(directoryInput('documents')))
      .entries.map(({ name }) => name)).toContain('Ideas');
    expect((await second.listDirectory(directoryInput('documents')))
      .entries.map(({ name }) => name)).not.toContain('Ideas');
  });
});
```

- [ ] **Step 2: Run the new frontend tests and observe missing modules**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files/files-view-state.spec.ts \
  --include=src/app/desktop/apps/files/files-demo.store.spec.ts
```

Expected: FAIL because the typed Files modules do not exist.

- [ ] **Step 3: Define readonly provider-neutral models**

Use discriminated unions and the Go wire names:

```ts
export type FilesDataState =
  | 'demo'
  | 'loading'
  | 'live'
  | 'stale'
  | 'unavailable';
export type FilesViewMode = 'grid' | 'list';
export type FileEntryKind = 'file' | 'directory' | 'link' | 'other';

export type FilesLocation =
  | { readonly kind: 'home' }
  | { readonly kind: 'trash' }
  | {
      readonly kind: 'directory';
      readonly mountId: string;
      readonly path: string;
    };

export interface FileAddressView {
  readonly mountId: string;
  readonly path: string;
}

export interface FilesCapabilities {
  readonly list: boolean;
  readonly preview: boolean;
  readonly createDirectory: boolean;
  readonly write: boolean;
  readonly rename: boolean;
  readonly copyFrom: boolean;
  readonly copyTo: boolean;
  readonly move: boolean;
  readonly trash: boolean;
  readonly restore: boolean;
  readonly delete: boolean;
}

export interface FileMountView {
  readonly id: string;
  readonly name: string;
  readonly kind: string;
  readonly icon: string;
  readonly brand: boolean;
  readonly capabilities: FilesCapabilities;
  readonly capacity?: {
    readonly freeBytes: number;
    readonly totalBytes: number;
  };
}

export interface FileEntryView {
  readonly name: string;
  readonly relativePath: string;
  readonly kind: FileEntryKind;
  readonly sizeBytes: number;
  readonly modifiedAt: string;
  readonly mode: number;
  readonly hidden: boolean;
}
```

Add equally strict readonly interfaces for catalogue, recent, breadcrumb,
directory, preview, trash, operation result, conflict, event, and every input.
Use one `FilesActionIntent` discriminated union for view outputs. Do not add
provider-specific fields or any absolute-path field.

- [ ] **Step 4: Implement pure navigation and reconciliation**

Use these stable token rules:

- `home` and `trash` are virtual routes;
- a mount root is its existing mount ID, preserving current deep links;
- a nested path is `<mount-id>::<encodeURIComponent(relative-path)>`;
- malformed percent encoding, backslashes, absolute paths, controls, `.`/`..`,
  or an unknown token shape returns Home; and
- `reconcileLocation(location, catalogue)` returns Home if the live mount no
  longer exists.

`buildFilesViewState` maps Home to registered mounts plus recents, Directory to
the current snapshot, and Trash to the trash snapshot. It computes count
labels, icons, selected-provider label, optional capacity label, Up, and
breadcrumbs without provider branching.

- [ ] **Step 5: Port—not rewrite—the complete existing fixture**

Move every `FS` row from `desktop.data.ts` into typed mount/path entries:

- `home` loose files become deterministic recent/demo-home entries;
- `documents/Invoices`, `projects/lethean`, and `projects/core-ide` retain
  their nesting;
- Downloads, Models, Projects, LetherNet, and empty Trash remain;
- names, sizes, modified labels, and icons remain visually equivalent; and
- demo capacity remains `218 GB free of 512 GB`, explicitly sourced as demo.

Give the LetherNet demo mount `kind: "memory"` and an appropriate icon; the
fixture must not imply that a network credential or provider exists.

`FilesDemoStore` implements the same high-level method names as the live bridge:
catalogue, list, preview, create, rename, copy, move, trash, listTrash, restore,
and delete. It clones the constant seed in its constructor, uses a monotonic
demo operation ID, applies the same path/capability/conflict rules, and never
imports Wails, fetches, accesses browser storage, or calls a host API.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files/files-view-state.spec.ts \
  --include=src/app/desktop/apps/files/files-demo.store.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/apps/files
git commit -m "feat(frontend): type Files state and demo data"
```

---

### Task 11: Strict live Files bridge with an offline and path-leak guard

**Files:**

- Create:
  `frontend-ng/src/app/desktop/desktop-files-bridge.service.ts`
- Create:
  `frontend-ng/src/app/desktop/desktop-files-bridge.service.spec.ts`

**Interfaces:**

- Consumes: `SurfaceBridgeService`, `ConnectionManagerService`, and Wails
  `Events`.
- Produces one typed method per Go Files method plus an explicit event
  subscription.

- [ ] **Step 1: Write failing bridge method, parser, event, and offline tests**

```ts
describe('DesktopFilesBridgeService', () => {
  it('calls the provider-neutral list method with mount and relative path', async () => {
    surface.call.mockResolvedValue(directoryWireFixture());

    await expect(service.listDirectory({
      mountId: 'documents',
      path: 'Invoices',
      cursor: '',
      limit: 200,
    })).resolves.toMatchObject({ path: 'Invoices' });

    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/office/files.Service.ListDirectory',
      [{ mountId: 'documents', path: 'Invoices', cursor: '', limit: 200 }],
    );
  });

  it('rejects absolute, traversal, and provider-leaking responses', async () => {
    for (const payload of [
      { ...directoryWireFixture(), path: '/Users/sarah/Documents' },
      { ...directoryWireFixture(), path: '../Documents' },
      { ...directoryWireFixture(), root: '/Users/sarah' },
      { ...directoryWireFixture(), endpoint: 's3://private-bucket' },
    ]) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(service.listDirectory(directoryInput('documents')))
        .rejects.toThrow('invalid Files response');
    }
  });

  it('makes no Wails call or event registration while explicitly offline', async () => {
    offline.set(true);

    await expect(service.listMounts()).rejects.toThrow('offline demo mode');
    const off = service.onChanged(vi.fn());

    expect(surface.call).not.toHaveBeenCalled();
    expect(events.on).not.toHaveBeenCalled();
    expect(off).toBeTypeOf('function');
  });
});
```

Add parser tests for every DTO and mutation method, malformed `core.Result`
payloads, missing required fields, invalid enum values, negative sizes, an
invalid event, and unsubscribe.

- [ ] **Step 2: Run the bridge test and observe the missing service**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-files-bridge.service.spec.ts
```

Expected: FAIL because the service does not exist.

- [ ] **Step 3: Implement one method-name table and explicit online guard**

```ts
const FILES_SERVICE = 'dappco.re/lthn/desktop/pkg/office/files.Service';
const FILES_METHODS = {
  listMounts: `${FILES_SERVICE}.ListMounts`,
  listDirectory: `${FILES_SERVICE}.ListDirectory`,
  preview: `${FILES_SERVICE}.Preview`,
  createDirectory: `${FILES_SERVICE}.CreateDirectory`,
  rename: `${FILES_SERVICE}.Rename`,
  copy: `${FILES_SERVICE}.Copy`,
  move: `${FILES_SERVICE}.Move`,
  trash: `${FILES_SERVICE}.Trash`,
  listTrash: `${FILES_SERVICE}.ListTrash`,
  restore: `${FILES_SERVICE}.Restore`,
  delete: `${FILES_SERVICE}.Delete`,
} as const;
```

Every method calls `requireOnline()` before `SurfaceBridgeService.call`.
`onChanged(handler)` returns a no-op unsubscriber when
`connection.offline()` is true; otherwise it registers exactly
`lthn:files:changed`.

Put event access behind a `FILES_EVENT_SOURCE` injection token mirroring
`DEEP_LINK_EVENTS`, so jsdom tests never need a native runtime.

- [ ] **Step 4: Parse unknown values without trusting TypeScript casts**

Implement small `requiredRecord`, `requiredString`, `requiredBoolean`,
`requiredNumber`, `requiredArray`, and enum readers. Recursively reject
case-insensitive field names `root`, `absolutePath`, `endpoint`, `credential`,
`secret`, `key`, and `encryptionKey`. Validate every returned `mountId` and
relative path again:

```ts
function providerRelativePath(value: unknown, context: string): string {
  const path = requiredStringAllowEmpty(value, context);
  if (
    path.startsWith('/') ||
    path.includes('\\') ||
    path.split('/').some((part) => part === '.' || part === '..') ||
    /^[A-Za-z]:/.test(path)
  ) {
    throw new Error(`The ${context} contains an invalid Files response.`);
  }
  return path;
}
```

Do not silently default a missing capability to `true`, missing arrays to a
fixture, or invalid capacity to zero.

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-files-bridge.service.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/desktop-files-bridge.service.ts \
  frontend-ng/src/app/desktop/desktop-files-bridge.service.spec.ts
git commit -m "feat(frontend): add strict Files live bridge"
```

---

### Task 12: Extract the Files presentation views without changing its visual contract

**Files:**

- Create:
  `frontend-ng/src/app/desktop/apps/files/files-sidebar.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-toolbar.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-browser.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-status.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-preview.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-operation-dialog.view.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files-views.spec.ts`
- Create:
  `frontend-ng/src/app/desktop/apps/files/files.app.scss`
- Modify: `frontend-ng/src/app/desktop/apps/files.app.ts` — attach the moved
  stylesheet to the still-working container before Task 13 rewrites it.
- Modify: `frontend-ng/src/app/desktop/desktop.component.scss`

**Interfaces:**

- Consumes only signal inputs from Task 10.
- Emits typed `FilesActionIntent` values.
- Does not inject bridge, live-data, window-manager, or provider services.

- [ ] **Step 1: Write failing isolated view contracts**

For each view, render fixed typed inputs and assert both presentation and
emitted intent. The suite must cover:

- sidebar group labels, active mount, Home, LetherNet, and Trash;
- Up, Home, breadcrumbs, Refresh, grid, and list toolbar outputs;
- empty, grid, and list browser states;
- single selection, directory open, file preview, and keyboard Enter;
- data-state badge, item/folder/file counts, provider label, and optional
  capacity;
- bounded text and binary preview states; and
- create/rename/copy/move/trash/restore/delete confirmation, conflict, partial,
  busy, success, and error feedback.

Example:

```ts
it('emits a directory-open intent without knowing a provider', async () => {
  const fixture = TestBed.createComponent(FilesBrowserView);
  fixture.componentRef.setInput('entries', [
    fileEntry({ name: 'Invoices', relativePath: 'Invoices', kind: 'directory' }),
  ]);
  const intents: FilesActionIntent[] = [];
  fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));
  fixture.detectChanges();

  fixture.nativeElement.querySelector<HTMLElement>('[data-path="Invoices"]')?.click();

  expect(intents).toEqual([{ type: 'open-directory', path: 'Invoices' }]);
});
```

- [ ] **Step 2: Run the isolated suite and observe missing components**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files/files-views.spec.ts
```

Expected: FAIL because the six views do not exist.

- [ ] **Step 3: Build standalone `OnPush` views**

Use Angular 22 `input.required`, `input`, and `output`; no `@Input`/`EventEmitter`
mix inside the new views. Give interactive rows real buttons or roving
`tabindex`, visible focus, Enter/Space behaviour, and stable ARIA labels.
Retain current `$localize` IDs wherever copy already exists and add new British
English IDs for Refresh, operation confirmations, conflict, stale, partial,
and preview labels.

`FilesPreviewView` renders escaped text through interpolation, never
`innerHTML`. Binary previews show metadata only. Link/other entries show an
unsupported message and no open action.

- [ ] **Step 4: Move the `.fb*` styling intact, then add only required states**

Copy the current `.fb`, `.fbside`, `.fbplace`, `.fbmain`, `.fbtop`, `.fbnav`,
`.fbcrumb`, `.fbvtog`, `.fbbody`, `.fblist`, `.fbrow`, `.fbgrid`, `.fbcell`,
`.fbempty`, and `.fbstatus` rules into `files.app.scss`. Remove only that block
from `desktop.component.scss`.

Attach the stylesheet to the current `FilesApp` in this same task with
`ViewEncapsulation.None` and `styleUrl: './files/files.app.scss'` before
removing the shell block, so the existing Files route never has an unstyled
intermediate commit. Its class contract spans extracted child views. Add styles
for selection, keyboard focus, stale state, preview, and operation dialog using
existing foundation tokens. Do not change fonts, colours, spacing, window
chrome, shell breakpoints, or unrelated desktop selectors.

- [ ] **Step 5: Run the view suite and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files/files-views.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/apps/files \
  frontend-ng/src/app/desktop/apps/files.app.ts \
  frontend-ng/src/app/desktop/desktop.component.scss
git commit -m "refactor(frontend): extract Files presentation views"
```

---

### Task 13: Rebuild `FilesApp` as a small live/demo read container

**Files:**

- Rewrite: `frontend-ng/src/app/desktop/apps/files.app.ts`
- Rewrite: `frontend-ng/src/app/desktop/apps/files.app.spec.ts`

**Interfaces:**

- Consumes: Task 10 state/demo, Task 11 bridge, Task 12 views,
  `WindowManagerService`, and the existing `Win`.
- Initially wires Home, mount browsing, breadcrumbs, Up, Refresh, grid/list,
  selection, and Preview. Mutations/events arrive in Task 14.

- [ ] **Step 1: Replace the old aggregate tests with failing container contracts**

Cover:

1. explicit offline mode shows the complete labelled demo and makes no bridge
   or event call;
2. connected start loads `ListMounts`, then the current mount/path;
3. a legacy `win.sub === "documents"` opens the Documents root;
4. a nested token opens the exact provider-relative path;
5. directory selection updates `win.sub` with a reversible token;
6. Up and breadcrumb navigation preserve mount and relative path;
7. file open calls Preview with mount/path and renders bounded content;
8. grid/list calls only `setSysTab`;
9. an out-of-order earlier request cannot replace a later navigation result;
10. live failure retains the last successful snapshot as stale, or shows
    unavailable when none exists; and
11. live failure never substitutes demo fixture values.

- [ ] **Step 2: Run the container spec and observe old-contract failures**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files.app.spec.ts
```

Expected: FAIL because `FilesApp` still uses `DesktopLiveDataService` and the
flat `FS` fixture.

- [ ] **Step 3: Implement a presentation-only template composition**

`FilesApp` imports the six views and renders:

```html
<div class="fb">
  <lthn-files-sidebar-view
    [state]="viewState()"
    (intent)="handleIntent($event)"
  />
  <main class="fbmain">
    <lthn-files-toolbar-view
      [state]="viewState()"
      (intent)="handleIntent($event)"
    />
    <lthn-files-browser-view
      [state]="viewState()"
      (intent)="handleIntent($event)"
    />
    <lthn-files-status-view [state]="viewState()" />
  </main>
  <lthn-files-preview-view
    *ngIf="preview() as filePreview"
    [preview]="filePreview"
    (intent)="handleIntent($event)"
  />
  <lthn-files-operation-dialog-view
    *ngIf="dialog() as operationDialog"
    [dialog]="operationDialog"
    (intent)="handleIntent($event)"
  />
</div>
```

Keep `ChangeDetectionStrategy.OnPush`, `CUSTOM_ELEMENTS_SCHEMA`,
`host: { style: 'display: contents' }`,
`ViewEncapsulation.None`, and `styleUrl: './files/files.app.scss'`.

- [ ] **Step 4: Implement deterministic load orchestration**

Store local signals for catalogue, location, directory/trash snapshot, preview,
selection, data state, last refresh, and failure. On initialisation:

- if `connection.offline()` is true, instantiate the per-window demo store,
  reconcile `win.sub`, load demo state synchronously/asynchronously, set
  `dataState = "demo"`, and return before touching the bridge;
- otherwise set loading, call `bridge.listMounts`, reconcile the token, and
  load the selected directory/Home;
- use an incrementing `loadVersion`; apply a response only when its captured
  version is still current; and
- on refresh failure, retain the last snapshot with `stale` or show
  `unavailable` with empty live state. Never instantiate the demo store as an
  error fallback.

Home is assembled from live mount catalogue and recent rows through
`buildFilesViewState`; it does not issue a host-home listing.

- [ ] **Step 5: Wire navigation, preview, and view mode**

- `navigate(location)` writes only `filesToken(location)` through
  `wm.setSub(win.id, token)`, clears selection/preview, and loads the new state.
- Opening a directory derives the current mount/path; it never concatenates an
  absolute path.
- Opening a file calls the active live bridge or demo store `preview`.
- `up` and breadcrumbs are taken from pure state, not reconstructed from labels.
- Grid/list continues to use `win.systab` and
  `wm.setSysTab(win.id, "grid" | "list")`.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files.app.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/apps/files.app.ts \
  frontend-ng/src/app/desktop/apps/files.app.spec.ts
git commit -m "feat(frontend): browse Medium mounts in Files"
```

---

### Task 14: Wire safe UI mutations, demo mutations, refresh events, and read-only WebMCP

**Files:**

- Modify: `frontend-ng/src/app/desktop/apps/files.app.ts`
- Modify: `frontend-ng/src/app/desktop/apps/files.app.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/app-mcp.spec.ts`

**Interfaces:**

- Consumes every bridge/demo operation and `lthn:files:changed`.
- Produces capability-aware visible actions, truthful operation feedback, and
  the three existing read-only WebMCP tools.

- [ ] **Step 1: Write failing operation, event, and WebMCP tests**

Add container tests for:

- action visibility follows the active mount's capabilities;
- Create Folder and Rename send parent/path plus a single validated name;
- Copy and Move collect a destination mount/path without an overwrite flag;
- Trash requires confirmation and refreshes Home/current mount/Trash;
- Restore and permanent Delete operate from the Trash view;
- recursive Delete requires its second explicit confirmation;
- `OperationConflict` keeps both addresses and leaves state unchanged;
- `OperationPartial` displays a persistent warning and refreshes both mounts;
- provider failures display unavailable/error state without demo fallback;
- connected mode subscribes once, refreshes only relevant events, coalesces an
  event plus local-success refresh, and unsubscribes on destroy;
- explicit offline mode registers no event listener and performs the same safe
  actions only against its per-window demo store; and
- event payloads with an unknown mount/path are ignored.

Update `app-mcp.spec.ts` to assert the exact existing tool names:

```ts
expect([...registered.keys()].sort()).toEqual([
  'files_navigate',
  'files_read_location',
  'files_set_view',
]);
expect([...registered.keys()].some((name) =>
  /create|rename|copy|move|trash|restore|delete/i.test(name),
)).toBe(false);
```

`files_read_location` must return the current token, mount ID, relative path,
breadcrumbs, view, data state, and visible provider-neutral entries.
`files_navigate` accepts a token returned by the read tool and validates it
against the current catalogue. `files_set_view` retains `grid | list`.
Register these global tool names only when `win.app === "files"`; the Office
wrapper in Task 15 reuses the UI but must not create a second registration.

- [ ] **Step 2: Run the container and MCP suites and observe failures**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files.app.spec.ts \
  --include=src/app/desktop/apps/app-mcp.spec.ts
```

Expected: FAIL because mutations, event refresh, and new token semantics are
not wired.

- [ ] **Step 3: Centralise action orchestration**

Implement one `handleIntent(intent: FilesActionIntent)` switch in the
container. It may open a typed dialog, navigate, preview, refresh, or call
`performOperation`; it must not contain provider-kind branches.

`performOperation`:

1. selects the demo store only when explicitly offline, otherwise the bridge;
2. sets a typed busy dialog;
3. calls the matching method with mount IDs and relative paths;
4. branches on `completed`, `conflict`, or `partial`;
5. never applies an optimistic destructive mutation to live data;
6. refreshes affected live/demo locations after completed or partial outcomes;
   and
7. keeps conflict/partial/error feedback visible until dismissed or retried.

Frontend name checks improve feedback, but Go remains the security boundary.
Never offer an action when capability is false; never assume that makes an
unauthorised backend call safe.

- [ ] **Step 4: Subscribe only in connected mode and coalesce refreshes**

After a successful connected initial load:

```ts
this.filesEventsOff = this.bridge.onChanged((event) => {
  if (eventAffectsLocation(event, this.location())) {
    this.scheduleRefresh();
  }
});
```

Register the unsubscriber with `DestroyRef`. `scheduleRefresh` queues at most
one refresh per microtask and increments the same `loadVersion` used in Task
13. Home refreshes for any known mount event; Trash refreshes for
trash/restore/delete; a directory refreshes when an affected path is itself,
its direct child, or its ancestor.

Provider-native watchers can emit the same event later; do not add polling in
this task.

- [ ] **Step 5: Retain read-only WebMCP without granting mutation authority**

Register the same three tools once. Keep `location_id` as the public argument
name for backwards compatibility, but its value is now the reversible token.
Tool execution delegates to the container's pure/read methods and
`WindowManagerService`; it never calls a mutation bridge method.

Reject unknown tokens with a calm error listing currently available root
tokens. Nested tokens can be used only after the tool has observed them in
`files_read_location` output.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/files.app.spec.ts \
  --include=src/app/desktop/apps/app-mcp.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/apps/files.app.ts \
  frontend-ng/src/app/desktop/apps/files.app.spec.ts \
  frontend-ng/src/app/desktop/apps/app-mcp.spec.ts
git commit -m "feat(frontend): wire safe Files operations"
```

---

### Task 15: Remove frontend duplication and make both routes use canonical Files

**Files:**

- Modify: `frontend-ng/src/app/desktop/desktop-live-data.service.ts`
- Modify: `frontend-ng/src/app/desktop/desktop-live-data.service.spec.ts`
- Modify: `frontend-ng/src/app/desktop/desktop.data.ts`
- Rewrite: `frontend-ng/src/app/desktop/surfaces/office/files.ts`
- Modify: `frontend-ng/src/app/desktop/surfaces/agents/code.ts`
- Modify: `frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts`
- Modify: `frontend-ng/src/app/desktop/index.ts` only if its export path changes.

**Interfaces:**

- Removes the `ListLocations`/`ListRecent`/`GetDiskUsage` aggregate and `FS`.
- Keeps both the app-shell Files route and Office catalogue route, but both
  render the same canonical component.

- [ ] **Step 1: Write the failing route-reuse and absence tests**

In `surface-registry.spec.ts`, resolve `surface-office-files`, render it with a
demo `Win`, and assert it contains `lthn-files-app`, the Demo data badge,
Documents, and `welcome.txt`. Assert it does not contain the old
`lthn-surface-page` fixture metrics or `~/Documents · 5 recent`.

In `desktop-live-data.service.spec.ts`, remove the old aggregate behaviour test
and assert `('files' in service) === false` after the method is removed. Add a
desktop-data assertion that `('fs' in DEFAULT_DESKTOP_DATA) === false`.
The explicit `rg` inspection in Step 3 proves the retired symbol names are
absent without making a browser test read repository source files.

- [ ] **Step 2: Run the affected tests and observe duplicate-fixture failures**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-live-data.service.spec.ts \
  --include=src/app/desktop/surfaces/surface-registry.spec.ts
```

Expected: FAIL while the retired aggregate and Office fixture still exist.

- [ ] **Step 3: Delete only Files-owned fixture/aggregate code**

- Remove `FileLocation`, `RecentFile`, `DiskUsage`, `FilesSnapshot`,
  `DesktopLiveDataService.files`, `parseFilesSnapshot`, and its helpers only
  when no other live-data method uses them.
- Remove `FsNode`, `FS`, `DesktopData.fs`, and
  `DEFAULT_DESKTOP_DATA.fs`.
- Do not touch telemetry, languages, control data, shell fixtures, DevPanel
  data, other surfaces, or `DESKTOP_DATA` consumers.

Run `rg -n '\b(FS|FsNode|FilesSnapshot|ListLocations|ListRecent|GetDiskUsage)\b'
frontend-ng/src/app/desktop` and inspect every remaining match. Expected
matches are only design/history prose if any; no TypeScript consumer remains.

- [ ] **Step 4: Make Office Files a thin canonical wrapper**

```ts
@Component({
  selector: 'lthn-office-files-surface',
  standalone: true,
  imports: [FilesApp],
  template: `<lthn-files-app [win]="win" />`,
  host: { style: 'display:contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class OfficeFilesSurface extends SurfaceRoute {}
```

Do not create a second bridge, store, fixture, poller, or copy of the Files
template. The route remains registered under `surface-office-files` so app
catalogue navigation is not removed.

Update the Agents Code explanatory copy from the retired absolute
`Files.Read` contract to `Files.Preview({mountId, path})`. Do not add an actual
bridge call there unless that surface has a registered repository mount and a
relative address.

- [ ] **Step 5: Run the route/data tests and commit**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-live-data.service.spec.ts \
  --include=src/app/desktop/surfaces/surface-registry.spec.ts \
  --include=src/app/desktop/apps/files.app.spec.ts
```

Expected: PASS.

```bash
git add frontend-ng/src/app/desktop/desktop-live-data.service.ts \
  frontend-ng/src/app/desktop/desktop-live-data.service.spec.ts \
  frontend-ng/src/app/desktop/desktop.data.ts \
  frontend-ng/src/app/desktop/surfaces/office/files.ts \
  frontend-ng/src/app/desktop/surfaces/agents/code.ts \
  frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts \
  frontend-ng/src/app/desktop/index.ts
git commit -m "refactor(frontend): retire duplicate Files fixtures"
```

If `frontend-ng/src/app/desktop/index.ts` is unchanged, omit it from `git add`.

---

### Task 16: Document the invariant, audit remaining debt, and run release proof

**Files:**

- Modify: `AGENTS.md`
- Modify: `TODO.md`
- Modify:
  `docs/superpowers/specs/2026-07-26-files-go-io-design.md`
- Create: `docs/security/io-medium-audit.md`

**Interfaces:**

- Makes `io.Medium` the explicit repository rule for new file-backed work.
- Records, but does not mechanically rewrite, pre-existing non-Files debt.
- Produces fresh Go, Angular, build, and visual verification evidence.

- [ ] **Step 1: Update the design/runtime-persistence statement**

Replace the design's permissive go-store paragraph with the verified current
constraint:

- the initial Files runtime document is serialised directly through a
  dedicated `io.Medium`;
- pinned/current go-store uses a structurally different legacy Medium contract
  and stages SQLite/workspace files on local paths internally;
- go-store is not part of this first strict Files implementation; and
- a future go-store use is acceptable only after every persistent byte and
  temporary stage is transported through an audited `io.Medium`.

This correction changes no product architecture: runtime metadata remains
separate from content mounts and may later change implementation behind the
same `RuntimeMetadata` interface.

- [ ] **Step 2: Add the invariant to `AGENTS.md`**

Under CoreGO development contract, add:

```text
All file-backed product operations must ultimately flow through a registered
dappco.re/go/io.Medium. io.Medium is the security boundary: do not add a raw
os/path/filepath/syscall/Core-Fs fallback for local convenience, including for
metadata, tests, previews, recursion, or error recovery. An unavailable Medium
fails closed. Resolve provider roots and credentials in trusted Go composition;
renderer contracts carry only mount IDs and provider-relative paths.
```

Also document the canonical Files package, runtime metadata path, default
mount IDs, internal namespace, Wails event name, offline-demo rule, and focused
test commands introduced by this plan.

- [ ] **Step 3: Make the Files backlog truthful**

In `TODO.md`:

- mark capability-scoped listing complete;
- mark bounded preview complete, but retain host open/reveal as separate work;
- mark create/rename/copy/move/trash/restore/delete complete;
- retain provider-native watch events as open;
- retain user-managed locations/removable/network discovery as open;
- mark pagination and base metadata complete while retaining search, thumbnail
  generation, richer MIME detection, and very-large catalogue indexing; and
- state that configured model-root migration waits on its own Medium-backed
  settings source.

Do not remove unrelated Control, Telemetry, Terminal, or shared-bridge work.

- [ ] **Step 4: Produce a reproducible repository-wide audit**

Run read-only inventories such as:

```bash
rg -n --glob '*.go' \
  '(^|[^[:alnum:]_])(os\\.(Open|ReadFile|WriteFile|Create|Remove|RemoveAll|Rename|Stat|ReadDir)|filepath\\.|syscall\\.|core\\.(ReadFile|WriteFile|ReadDir|DirFS|Stat|Lstat|MkdirAll))' \
  go
rg -n --glob '*.ts' \
  '(absolutePath|file://|Files\\.Read|ListLocations|ListRecent|GetDiskUsage)' \
  frontend-ng/src
```

Write `docs/security/io-medium-audit.md` with:

- command, date, commit, and scope;
- one row per affected package grouped by data ownership;
- current access mechanism;
- target Medium/mount owner;
- risk (`boundary`, `metadata`, `build tooling`, or `test only`);
- migration order; and
- explicit statement that Files is guarded but the repository is not yet
  globally sealed.

Do not turn this audit step into thousands of unrelated rewrites.

- [ ] **Step 5: Run focused Go proof**

```bash
cd /Users/snider/Code/core/go-io/go
go test ./local -count=1
go test ./... -count=1
go vet ./...

cd /Users/snider/Lethean/agent/cladius/Code/lthn/desktop
go test ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/office/files ./go/pkg/desktop ./go/cmd/lthn
gofmt -l go/pkg/office/files go/pkg/desktop/files_events.go \
  go/pkg/desktop/files_events_test.go
```

Expected: tests/vet PASS and `gofmt -l` prints nothing.

- [ ] **Step 6: Run the Angular confidence gate and production build**

```bash
cd /Users/snider/Lethean/agent/cladius/Code/lthn/desktop
wails3 task verify:frontend
cd frontend-ng
npm run build
```

Expected: ordered lint/type/test confidence gate PASS and production output at
`../go/cmd/lthn/dist/index.html`.

- [ ] **Step 7: Run browser demo visual and interaction proof**

Start:

```bash
cd frontend-ng
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
```

Inspect all three deterministic URLs:

```text
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=desktop#/
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=shell#/
http://127.0.0.1:9245/?lthn-offline=1&lthn-view=device&lthn-device=small#/
```

Verify:

- fonts and existing dark-calm tokens still load;
- sidebar, Home, nested folders, Up, breadcrumbs, grid/list, empty state,
  status, capacity, preview, and every confirmation dialog remain usable;
- demo mutations change the visible catalogue and never create a Wails socket
  or Wails event listener;
- resize across mobile/tablet/desktop preserves controls without clipping; and
- console has no uncaught errors or accessibility-name regressions.

Capture screenshots only as verification artefacts; do not add them to the
repository unless the owner asks.

- [ ] **Step 8: Run native connected smoke**

With the browser server stopped and port `9099` free:

```bash
cd /Users/snider/Lethean/agent/cladius/Code/lthn/desktop
wails3 task dev
```

Verify one real temporary/approved mount path through the native app:
ListMounts, root/nested listing, bounded preview, create/rename, copy, trash,
restore, and delete. Confirm the Angular network/event log contains only mount
IDs and relative paths and receives `lthn:files:changed`. Do not use a personal
document as a destructive test target; create a dedicated disposable folder
through the Files UI.

- [ ] **Step 9: Run repository diff and broad proportional checks**

```bash
cd /Users/snider/Lethean/agent/cladius/Code/lthn/desktop
git diff --check
git status --short
wails3 task test
```

Expected: `git diff --check` PASS. Record any pre-existing broad-suite/security
sweep failure separately with its exact command and output; do not weaken a
test or claim global CoreGO compliance to make the turn green.

- [ ] **Step 10: Commit documentation and audit evidence**

```bash
git add AGENTS.md TODO.md \
  docs/superpowers/specs/2026-07-26-files-go-io-design.md \
  docs/security/io-medium-audit.md
git commit -m "docs(files): codify the Medium security boundary"
```

---

## Plan self-review

- [x] **Specification coverage:** every design section maps to at least one
  task: invariant (1, 9, 16), upstream gate (7–9), read contract
  (2–3), mutations (4–6), Angular state/bridge/views (10–14), duplication
  retirement (9, 15), events (4, 9, 14), demo mode (10, 13–14), audit/docs
  (16), and verification (16).
- [x] **Independent checkpoints:** each task has a focused red/green command
  and a commit boundary. Tasks 7 and 8 are the sole paired upstream checkpoint
  and intentionally commit together.
- [x] **Type consistency:** Go and TypeScript both use
  `completed | conflict | partial`; mutation inputs contain only mount IDs,
  relative paths, names, receipt IDs, and explicit confirmation; no renderer
  type contains a provider root.
- [x] **Boundary consistency:** the plan covers raw-call guards, boolean
  decision guards, internal-namespace ownership, path-leak parsing, and
  error-to-missing failure cases.
- [x] **Demo/live consistency:** the plan proves offline creates neither Wails
  calls nor listeners, connected failure never loads demo state, and both data
  sources implement the same container-facing operations.
- [x] **Placeholder audit:** code fragments use concrete fields and bindings;
  no implementation choice or test body is left unassigned.
- [x] **Worktree safety:** every relevant commit scope excludes
  `.playwright-mcp/` and calls out the existing Desktop and upstream
  `go.work.sum` changes for preservation.

## Execution handoff

Execute this plan inline with `superpowers:executing-plans`, task by task, on
the existing Desktop `main` worktree and the existing upstream go-io `dev`
worktree. Do not dispatch sub-agents. Stop only at the explicit upstream release
checkpoint if tag/push authority has not already been granted; all work before
that checkpoint remains testable against Memory media.
