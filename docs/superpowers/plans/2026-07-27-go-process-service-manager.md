<!-- SPDX-License-Identifier: EUPL-1.2 -->

# `go-process` Service Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a manual-by-default Lethean Desktop service manager in `go/pkg/services`, backed by `dappco.re/go/process` and `io.Medium`, then make Control's existing Daemons tab a working Services interface.

**Architecture:** A Core-registered `services.Service` owns durable trusted definitions and runtime desired state. It delegates every child-process operation to the named `go-process.Service`, persists only catalogue data through the registered application `io.Medium`, forwards typed state invalidations through Wails, and exposes a defensive Angular bridge consumed by a focused Control presentation component.

**Tech Stack:** Go 1.26.2, CoreGO `dappco.re/go` v0.12.0, `dappco.re/go/process` v0.16.1, `dappco.re/go/io` v0.15.3, Wails 3 alpha2.117, Angular 22, TypeScript 6, Angular signals, Vitest/jsdom.

## Global Constraints

- Work on the current `main` branch; preserve the existing user-owned `go.work.sum` modification and `.playwright-mcp/` directory.
- Do not use subagents; execute inline because the user explicitly requested no subagent workflow.
- `frontend-ng/` is the only product frontend; retain hash routing and browser demo mode.
- No process starts during Core registration, `OnStartup`, catalogue reads, UI refreshes, or event subscription.
- Ordinary managed lifecycle must never call launchd, systemd, the legacy native controller, `os/exec`, or the package-global `go-process` runtime.
- Every managed spawn, observation, output read, graceful shutdown, and process-tree termination goes through the named `*dappco.re/go/process.Service`.
- Every new persisted byte, read, recovery, existence check, and rename goes through the registered application `io.Medium`; no raw filesystem fallback.
- Persist definitions and policy only; never persist desired-running state, PIDs, process IDs, output, environment, credentials, or raw errors.
- Renderer operations address a known service ID only; they do not accept arbitrary command, argument, environment, or absolute working-directory values.
- Use British English in code, copy, docs, and tests.
- Use real red-green Good/Bad/Ugly tests, `*core.T`, `io.NewMemoryMedium()`, and deterministic fakes; never write to the real `~/Lethean/` tree.
- Keep the legacy native `lthn service install` packaging path isolated and compatible, but do not use it for the Angular Services view.
- Explicit Desktop shutdown stops managed services; closing all windows leaves them running while the tray-owned Core remains alive.
- Offline Angular demo mode makes no Wails calls or event subscriptions and keeps all demo values visibly labelled.

---

## File Map

### Go manager

- Create `go/pkg/services/types.go` — wire-safe definition, snapshot, output, event, limit, state, policy, and typed failure contracts.
- Create `go/pkg/services/types_test.go` — definition validation, cloning, limits, error-code, and secret-shape tests.
- Create `go/pkg/services/types_example_test.go` — runnable examples for the new public types and validation.
- Create `go/pkg/services/catalogue.go` — `Catalogue` interface plus atomic/recoverable `io.Medium` implementation.
- Create `go/pkg/services/catalogue_test.go` — Memory/failing-Medium persistence and recovery tests.
- Create `go/pkg/services/catalogue_example_test.go` — Medium catalogue example.
- Create `go/pkg/services/process_runtime.go` — narrow adapters over `go-process.Service` and `go-process.Process`.
- Create `go/pkg/services/service.go` — construction, Core registration method, catalogue load/merge, reads, trusted definition registration, and policy changes.
- Create `go/pkg/services/lifecycle.go` — start, stop, restart, exit observation, restart backoff, output, and shutdown.
- Create `go/pkg/services/events.go` — Core ACTION event and audit emission.
- Create `go/pkg/services/service_test.go` — startup, catalogue, start/stop, output, and concurrency contracts.
- Create `go/pkg/services/restart_test.go` — restart policy, generation, backoff, cancellation, and shutdown contracts.
- Create `go/pkg/services/service_example_test.go` — canonical construction and lifecycle examples.
- Modify `go/pkg/services/wails.go` — bind the registered manager; retain explicit native compatibility operations.
- Replace scaffold-only coverage in `go/pkg/services/wails_test.go` with behavioural delegation tests.
- Modify `go/pkg/services/wails_example_test.go` — examples with the new constructor.
- Modify `go/pkg/audit/types.go` — typed service lifecycle audit names.

### Composition and native event forwarding

- Modify `go/cmd/lthn/app.go` — register `services` after `process` and `io`.
- Modify `go/pkg/desktop/desktop.go` — resolve the registered manager, bind its Wails wrapper, and install event forwarding.
- Create `go/pkg/desktop/services_events.go` — map `services.Event` to `lthn:services:changed`.
- Create `go/pkg/desktop/services_events_test.go` — bounded event-forwarding proof.
- Modify `go/cmd/lthn/app_test.go` — assert the named `services` registration
  follows `process` and `io` without starting a child process.

### Angular bridge and Control interface

- Create `frontend-ng/src/app/desktop/desktop-services-bridge.service.ts` — Wails calls, strict parsing, offline guard, and event source.
- Create `frontend-ng/src/app/desktop/desktop-services-bridge.service.spec.ts` — wire, parser, mutation, secret-shape, and event tests.
- Create `frontend-ng/src/app/desktop/apps/control/control-services.models.ts` — typed view state, intents, and deterministic demo catalogue.
- Create `frontend-ng/src/app/desktop/apps/control/control-services.view.ts` — focused Services presenter with lifecycle actions and output details.
- Create `frontend-ng/src/app/desktop/apps/control/control-services.view.spec.ts` — presenter rendering and intent tests.
- Modify `frontend-ng/src/app/desktop/apps/control/control-system.view.ts` — retain `daemons` state value, render the new presenter, visible label Services.
- Modify `frontend-ng/src/app/desktop/apps/control/control-secondary-views.spec.ts` — integration seam for the Services tab.
- Modify `frontend-ng/src/app/desktop/apps/control.app.ts` — own demo/live resource, mutation orchestration, event invalidation, and teardown.
- Modify `frontend-ng/src/app/desktop/apps/control.app.spec.ts` — connected/demo lifecycle and action tests.

### Documentation and contracts

- Modify `TODO.md` — mark the typed daemon/service registry item complete only after all behaviour is verified.
- Modify `AGENTS.md` — add the verified manager paths, manual-start contract, and focused checks.

---

### Task 1: Define the trusted service catalogue contract

**Files:**
- Create: `go/pkg/services/types.go`
- Create: `go/pkg/services/types_test.go`
- Create: `go/pkg/services/types_example_test.go`

**Interfaces:**
- Consumes: `core.Result`, `core.E`, `core.Now`, and JSON conventions from `dappco.re/go`.
- Produces: `Kind`, `RestartPolicy`, `State`, `ErrorCode`, `Definition`, `DefinitionView`, `PolicyOverride`, `Snapshot`, `CatalogueView`, `OutputRequest`, `OutputView`, `Event`, `Limits`, `Failure`, `ValidateDefinition`, `DefaultLimits`, and clone helpers.

- [ ] **Step 1: Write failing Good/Bad/Ugly contract tests**

```go
func TestValidateDefinition_Good(t *core.T) {
    definition := validDefinition()
    result := ValidateDefinition(definition, DefaultLimits())
    core.AssertTrue(t, result.OK)
}

func TestValidateDefinition_BadRejectsCommandlessOrUnknownPolicy(t *core.T) {
    definition := validDefinition()
    definition.Command = ""
    core.AssertFalse(t, ValidateDefinition(definition, DefaultLimits()).OK)

    definition = validDefinition()
    definition.RestartPolicy = RestartPolicy("sometimes")
    core.AssertFalse(t, ValidateDefinition(definition, DefaultLimits()).OK)
}

func TestValidateDefinition_UglyRejectsRendererPathAndSecretEnvironment(t *core.T) {
    definition := validDefinition()
    definition.WorkingDirectory = WorkingDirectory{MountID: "projects", Path: "/Users/sarah/Code"}
    result := ValidateDefinition(definition, DefaultLimits())
    core.AssertFalse(t, result.OK)
    core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
}

func TestSnapshotClone_GoodDoesNotAliasDefinitionSlices(t *core.T) {
    snapshot := Snapshot{Definition: validDefinitionView(), ProcessID: "proc-1"}
    clone := cloneSnapshot(snapshot)
    core.AssertEqual(t, snapshot, clone)
}
```

- [ ] **Step 2: Run the focused tests and confirm the red state**

Run:

```bash
go test ./go/pkg/services -run 'TestValidateDefinition|TestSnapshotClone' -count=1
```

Expected: compile failure because the new contracts do not exist.

- [ ] **Step 3: Implement the typed model and stable failures**

```go
type Kind string
type RestartPolicy string
type State string
type ErrorCode string

const (
    KindService Kind = "service"
    KindApp     Kind = "app"
    KindProcess Kind = "process"

    RestartNever     RestartPolicy = "never"
    RestartOnFailure RestartPolicy = "on-failure"
    RestartAlways    RestartPolicy = "always"

    StateStopped  State = "stopped"
    StateStarting State = "starting"
    StateRunning  State = "running"
    StateStopping State = "stopping"
    StateExited   State = "exited"
    StateFailed   State = "failed"

    ErrorServicesUnavailable       ErrorCode = "services_unavailable"
    ErrorCatalogueInvalid          ErrorCode = "catalogue_invalid"
    ErrorDefinitionNotFound        ErrorCode = "definition_not_found"
    ErrorDefinitionInvalid         ErrorCode = "definition_invalid"
    ErrorDefinitionConflict        ErrorCode = "definition_conflict"
    ErrorOperationInProgress       ErrorCode = "operation_in_progress"
    ErrorWorkingDirectoryUnsupported ErrorCode = "working_directory_unsupported"
    ErrorRunningLimitReached       ErrorCode = "running_limit_reached"
    ErrorProcessStartFailed        ErrorCode = "process_start_failed"
    ErrorProcessLookupFailed       ErrorCode = "process_lookup_failed"
    ErrorProcessStopFailed         ErrorCode = "process_stop_failed"
    ErrorRestartBudgetExhausted    ErrorCode = "restart_budget_exhausted"
    ErrorShutdownIncomplete        ErrorCode = "shutdown_incomplete"
)

type WorkingDirectory struct {
    MountID string `json:"mountId,omitempty"`
    Path    string `json:"path,omitempty"`
}

type Definition struct {
    ID                  string           `json:"id"`
    DisplayName         string           `json:"displayName"`
    Description         string           `json:"description"`
    Kind                Kind             `json:"kind"`
    Command             string           `json:"command"`
    Arguments           []string         `json:"arguments,omitempty"`
    WorkingDirectory    WorkingDirectory `json:"workingDirectory,omitempty"`
    RestartPolicy       RestartPolicy    `json:"restartPolicy"`
    GracePeriodMillis   int64            `json:"gracePeriodMillis"`
    Owner               string           `json:"owner"`
}

type DefinitionView struct {
    ID                string        `json:"id"`
    DisplayName       string        `json:"displayName"`
    Description       string        `json:"description"`
    Kind              Kind          `json:"kind"`
    RestartPolicy     RestartPolicy `json:"restartPolicy"`
    GracePeriodMillis int64         `json:"gracePeriodMillis"`
    Owner             string        `json:"owner"`
}

type PolicyOverride struct {
    ID                  string        `json:"id"`
    RestartPolicy       RestartPolicy `json:"restartPolicy"`
    GracePeriodMillis   int64         `json:"gracePeriodMillis"`
}

type FailureView struct {
    Code    ErrorCode `json:"code"`
    Message string    `json:"message"`
}

type Snapshot struct {
    Definition   DefinitionView `json:"definition"`
    State        State          `json:"state"`
    Desired      bool           `json:"desired"`
    ProcessID    string         `json:"processId"`
    PID          int            `json:"pid"`
    StartedAt    string         `json:"startedAt"`
    StoppedAt    string         `json:"stoppedAt"`
    ExitCode     int            `json:"exitCode"`
    RestartCount int            `json:"restartCount"`
    LastError    *FailureView   `json:"lastError"`
}

type CatalogueView struct {
    Services    []Snapshot `json:"services"`
    RefreshedAt string     `json:"refreshedAt"`
}

type OutputRequest struct {
    ID    string `json:"id"`
    Limit int    `json:"limit"`
}

type OutputView struct {
    ID         string `json:"id"`
    ProcessID  string `json:"processId"`
    Generation uint64 `json:"generation"`
    Output     string `json:"output"`
    Truncated  bool   `json:"truncated"`
    ObservedAt string `json:"observedAt"`
}

type Event struct {
    ID         string    `json:"id"`
    Operation  string    `json:"operation"`
    Previous   State     `json:"previous"`
    State      State     `json:"state"`
    Desired    bool      `json:"desired"`
    ProcessID  string    `json:"processId"`
    ErrorCode  ErrorCode `json:"errorCode"`
    At         string    `json:"at"`
}

type Limits struct {
    MaxDefinitions         int
    MaxArguments           int
    MaxArgumentBytes       int
    MaxRunning             int
    MaxOutputBytes         int
    MaxGracePeriodMillis   int64
    RestartLimit           int
    RestartWindow          time.Duration
    RestartBaseDelay       time.Duration
    RestartMaxDelay        time.Duration
    RestartExponentCap     int
}

type Failure struct {
    Code      ErrorCode
    Operation string
    Message   string
    Cause     error
}

func (f *Failure) Error() string
func (f *Failure) Unwrap() error
func ErrorCodeOf(result core.Result) ErrorCode
func DefaultLimits() Limits
func ValidateDefinition(definition Definition, limits Limits) core.Result
```

Validation uses a lower-case ID expression equivalent to
`^[a-z0-9][a-z0-9._-]{0,63}$`, allows only the three closed-set kinds and
policies, requires non-empty display name/command/owner, bounds arguments and
grace period through `Limits`, and rejects absolute, traversal, NUL, or
control-bearing working-directory paths. `DefinitionView` deliberately omits
command, arguments, and working-directory details.

- [ ] **Step 4: Add runnable examples and run the package tests**

```go
func ExampleValidateDefinition() {
    result := services.ValidateDefinition(services.Definition{
        ID: "api", DisplayName: "Lethean API", Kind: services.KindService,
        Command: "lthn", Arguments: []string{"serve"},
        RestartPolicy: services.RestartNever, GracePeriodMillis: 5_000,
        Owner: "lethean",
    }, services.DefaultLimits())
    core.Println(result.OK)
    // Output: true
}
```

Run:

```bash
go test ./go/pkg/services -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the contract**

```bash
git add go/pkg/services/types.go go/pkg/services/types_test.go go/pkg/services/types_example_test.go
git commit -m "feat(services): define managed service contracts"
```

---

### Task 2: Persist the catalogue exclusively through `io.Medium`

**Files:**
- Create: `go/pkg/services/catalogue.go`
- Create: `go/pkg/services/catalogue_test.go`
- Create: `go/pkg/services/catalogue_example_test.go`

**Interfaces:**
- Consumes: `Definition`, `PolicyOverride`, `ValidateDefinition`, `Limits`, and `coreio.Medium`.
- Produces:

```go
type CatalogueDocument struct {
    Version         int              `json:"version"`
    Definitions     []Definition     `json:"definitions"`
    PolicyOverrides []PolicyOverride `json:"policyOverrides"`
    UpdatedAt       string           `json:"updatedAt"`
}

type Catalogue interface {
    Load() core.Result
    Save(CatalogueDocument) core.Result
}

func NewMediumCatalogue(medium coreio.Medium, relativePath string, limits Limits) Catalogue
```

- [ ] **Step 1: Write failing Medium round-trip and recovery tests**

```go
func TestMediumCatalogue_GoodRoundTrip(t *core.T) {
    medium := coreio.NewMemoryMedium()
    catalogue := NewMediumCatalogue(medium, "desktop/services/catalogue.json", DefaultLimits())
    document := CatalogueDocument{
        Version: CatalogueVersion,
        Definitions: []Definition{validDefinition()},
        UpdatedAt: "2026-07-27T12:00:00Z",
    }

    core.AssertTrue(t, catalogue.Save(document).OK)
    loaded := catalogue.Load()
    core.AssertTrue(t, loaded.OK)
    core.AssertEqual(t, document, loaded.Value.(CatalogueDocument))
    core.AssertTrue(t, medium.IsFile("desktop/services/catalogue.json"))
}

func TestMediumCatalogue_BadMalformedDocumentFailsClosed(t *core.T) {
    medium := coreio.NewMemoryMedium()
    core.RequireNoError(t, medium.Write("desktop/services/catalogue.json", `{"version":1`))
    result := NewMediumCatalogue(
        medium, "desktop/services/catalogue.json", DefaultLimits(),
    ).Load()
    core.AssertFalse(t, result.OK)
    core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(result))
}

func TestMediumCatalogue_UglyRecoversValidatedBackup(t *core.T) {
    medium := coreio.NewMemoryMedium()
    core.RequireNoError(t, medium.EnsureDir("desktop/services/.staging"))
    core.RequireNoError(t, medium.Write(
        "desktop/services/.staging/catalogue.backup.json",
        validCatalogueJSON(t),
    ))
    loaded := NewMediumCatalogue(
        medium, "desktop/services/catalogue.json", DefaultLimits(),
    ).Load()
    core.AssertTrue(t, loaded.OK)
    core.AssertTrue(t, medium.IsFile("desktop/services/catalogue.json"))
}
```

Add a failing-Medium fixture which embeds `coreio.Medium` and overrides
`Read`, `WriteMode`, `Rename`, `Delete`, `EnsureDir`, `List`, or `Stat` to
return `fs.ErrPermission`. Assert every failure remains non-OK and never
creates an alternate path.

- [ ] **Step 2: Run the catalogue tests and confirm failure**

Run:

```bash
go test ./go/pkg/services -run 'TestMediumCatalogue' -count=1
```

Expected: compile failure because `Catalogue` is not defined.

- [ ] **Step 3: Implement versioned load, atomic save, and backup recovery**

Use these fixed provider-relative paths derived from the configured target:

```go
const CatalogueVersion = 1

type mediumCatalogue struct {
    medium      coreio.Medium
    path        string
    stagingDir  string
    stagedPath  string
    backupPath  string
    limits      Limits
    mu          sync.Mutex
}
```

`Save` must:

1. validate every definition and override;
2. marshal with `core.JSONMarshal`;
3. `EnsureDir` only the Medium-relative parent and staging directory;
4. `WriteMode(stagedPath, payload, 0o600)`;
5. read and decode the staged file again;
6. delete a stale backup through the Medium;
7. rename the current target to the backup when `Stat` proves it exists;
8. rename the staged file to the target;
9. restore the backup if the final rename fails; and
10. delete the validated backup after success.

`Load` must distinguish `fs.ErrNotExist` from provider failure. A genuinely
missing target with no backup returns a fresh version-1 document; malformed,
unsupported, or provider-failed reads return `ErrorCatalogueInvalid` or
`ErrorServicesUnavailable`. A valid backup may be renamed back only after
full decode and validation.

- [ ] **Step 4: Prove no host filesystem fallback**

Add a source-contract test which reads changed package source through an
`io.NewSandboxed(".")` test Medium and rejects new uses of:

```text
"os"
"path/filepath"
core.ReadFile(
core.WriteFile(
core.Stat(
core.Remove(
```

Exclude the pre-existing native adapter files `manager.go` and `registry.go`
from this new manager-data-plane assertion; the test must cover
`catalogue.go`, `service.go`, `lifecycle.go`, and `process_runtime.go`.

Run:

```bash
go test ./go/pkg/services -run 'TestMediumCatalogue|TestManagedServiceMediumBoundary' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Medium persistence**

```bash
git add go/pkg/services/catalogue.go go/pkg/services/catalogue_test.go \
  go/pkg/services/catalogue_example_test.go go/pkg/services/types_test.go
git commit -m "feat(services): persist catalogue through io Medium"
```

---

### Task 3: Add manual start, stop, status, and bounded output through `go-process`

**Files:**
- Create: `go/pkg/services/process_runtime.go`
- Create: `go/pkg/services/service.go`
- Create: `go/pkg/services/lifecycle.go`
- Create: `go/pkg/services/service_test.go`
- Create: `go/pkg/services/service_example_test.go`

**Interfaces:**
- Consumes: `Catalogue`, `Definition`, `Snapshot`, `Limits`, `coreprocess.RunOptions`.
- Produces:

```go
type ProcessHandle interface {
    Info() coreprocess.Info
    Output() string
    Wait() core.Result
    Shutdown() core.Result
}

type ProcessRuntime interface {
    StartWithOptions(core.Context, coreprocess.RunOptions) core.Result
    Get(string) core.Result
}

type WorkingDirectoryResolver interface {
    Resolve(WorkingDirectory) core.Result
}

func NewService(Options) *Service
func (s *Service) Register(*core.Core) core.Result
func (s *Service) OnStartup(core.Context) core.Result
func (s *Service) Catalogue() core.Result
func (s *Service) Get(string) core.Result
func (s *Service) EnsureDefinition(Definition) core.Result
func (s *Service) RemoveDefinition(owner, id string) core.Result
func (s *Service) SetPolicy(PolicyOverride) core.Result
func (s *Service) Start(string) core.Result
func (s *Service) Stop(string) core.Result
func (s *Service) Output(OutputRequest) core.Result
```

- [ ] **Step 1: Build deterministic process fakes and write failing lifecycle tests**

```go
type fakeProcess struct {
    info          coreprocess.Info
    output        string
    done          chan struct{}
    waitResult    core.Result
    shutdownCalls int
}

func (p *fakeProcess) Info() coreprocess.Info { return p.info }
func (p *fakeProcess) Output() string         { return p.output }
func (p *fakeProcess) Wait() core.Result {
    <-p.done
    return p.waitResult
}
func (p *fakeProcess) Shutdown() core.Result {
    p.shutdownCalls++
    if p.info.Running {
        p.info.Running = false
        p.info.Status = coreprocess.StatusKilled
        close(p.done)
    }
    return core.Ok(nil)
}

func TestService_OnStartup_GoodLoadsWithoutStarting(t *core.T) {
    fixture := newServiceFixture(t)
    core.AssertTrue(t, fixture.service.OnStartup(core.Background()).OK)
    core.AssertEqual(t, 0, len(fixture.runtime.starts))
}

func TestService_Start_GoodUsesGoProcessOptions(t *core.T) {
    fixture := newStartedServiceFixture(t)
    result := fixture.service.Start("api")
    core.AssertTrue(t, result.OK)
    core.RequireTrue(t, len(fixture.runtime.starts) == 1)
    options := fixture.runtime.starts[0]
    core.AssertEqual(t, fixture.definition.Command, options.Command)
    core.AssertEqual(t, fixture.definition.Arguments, options.Args)
    core.AssertTrue(t, options.Detach)
    core.AssertTrue(t, options.KillGroup)
    core.AssertEqual(t, 5*time.Second, options.GracePeriod)
}

func TestService_Stop_GoodDelegatesGracefulShutdown(t *core.T) {
    fixture := newRunningServiceFixture(t)
    result := fixture.service.Stop("api")
    core.AssertTrue(t, result.OK)
    core.AssertEqual(t, 1, fixture.process.shutdownCalls)
    core.AssertEqual(t, StateStopped, result.Value.(Snapshot).State)
}

func TestService_Output_UglyReturnsBoundedTail(t *core.T) {
    fixture := newRunningServiceFixture(t)
    fixture.process.output = "0123456789"
    result := fixture.service.Output(OutputRequest{ID: "api", Limit: 4})
    view := result.Value.(OutputView)
    core.AssertEqual(t, "6789", view.Output)
    core.AssertTrue(t, view.Truncated)
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run:

```bash
go test ./go/pkg/services -run 'TestService_(OnStartup|Start|Stop|Output)' -count=1
```

Expected: compile failure because the manager does not exist.

- [ ] **Step 3: Implement construction, catalogue merging, and trusted definition writes**

`Options` contains:

```go
type Options struct {
    Process                  ProcessRuntime
    Catalogue                Catalogue
    Builtins                 []Definition
    Limits                   Limits
    WorkingDirectoryResolver WorkingDirectoryResolver
    Now                      func() time.Time
    After                    func(time.Duration) <-chan time.Time
}
```

`OnStartup` loads once, validates the entire document, merges built-ins by ID,
applies policy overrides, creates stopped snapshots, and records an
unavailable failure without starting a process. `Catalogue` and lifecycle
operations return that stable failure until a valid reload; unrelated Core
services remain available.

`EnsureDefinition` and `RemoveDefinition` accept trusted Go calls, enforce
owner identity, update a cloned document, persist first, then publish the new
in-memory catalogue. They reject running or operation-in-progress entries.

- [ ] **Step 4: Implement explicit start, idempotent stop, reads, and output**

Store a private runtime record per ID:

```go
type runtimeRecord struct {
    snapshot      Snapshot
    process       ProcessHandle
    generation    uint64
    operation     bool
    restartCancel core.CancelFunc
    restartTimes  []time.Time
}
```

`Start` locks only to validate and mark `{Desired:true, StateStarting,
operation:true, generation++}`, then unlocks before resolving the working
directory and calling `StartWithOptions`. It accepts only a returned
`ProcessHandle`, snapshots its `Info`, commits running state for the same
generation, and starts an exit observer.

Use:

```go
coreprocess.RunOptions{
    Command:       definition.Command,
    Args:          append([]string(nil), definition.Arguments...),
    Dir:           resolvedDirectory,
    DisableCapture: false,
    Detach:        true,
    GracePeriod:   time.Duration(definition.GracePeriodMillis) * time.Millisecond,
    KillGroup:     true,
}
```

`Stop` clears desired state and cancels pending restart before external work,
calls `ProcessHandle.Shutdown`, then commits stopped state only when its
generation still matches. Already non-running states return their current
snapshot.

`Output` validates `1 <= limit <= Limits.MaxOutputBytes`, resolves only the
current `ProcessHandle`, slices the UTF-8-safe byte tail, and returns generation
metadata without persisting or emitting content.

- [ ] **Step 5: Run package tests and the race detector**

Run:

```bash
go test ./go/pkg/services -count=1
go test -race ./go/pkg/services -run 'TestService_(Start|Stop|Output)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the manual lifecycle**

```bash
git add go/pkg/services/process_runtime.go go/pkg/services/service.go \
  go/pkg/services/lifecycle.go go/pkg/services/service_test.go \
  go/pkg/services/service_example_test.go
git commit -m "feat(services): manage processes through go-process"
```

---

### Task 4: Add exit reconciliation, restart policy, events, audit, and shutdown

**Files:**
- Modify: `go/pkg/services/lifecycle.go`
- Create: `go/pkg/services/events.go`
- Create: `go/pkg/services/restart_test.go`
- Modify: `go/pkg/services/service_test.go`
- Modify: `go/pkg/audit/types.go`
- Create: `go/pkg/services/audit_test.go`

**Interfaces:**
- Consumes: Task 3's `runtimeRecord`, `ProcessHandle`, `Service`, and Core ACTION bus.
- Produces: `Service.Restart`, `Service.OnShutdown`, generation-bound exit observation, bounded restart backoff, `Subscribe`, typed service events, and audit lifecycle rows.

- [ ] **Step 1: Write failing restart, race, and shutdown tests**

```go
func TestService_RestartOnFailure_GoodRestartsDesiredService(t *core.T) {
    fixture := newRunningServiceFixtureWithPolicy(t, RestartOnFailure)
    fixture.process.info = exitedInfo(7)
    close(fixture.process.done)
    fixture.clock.releaseNextRestart()
    core.Eventually(t, func() bool { return len(fixture.runtime.starts) == 2 })
}

func TestService_RestartNever_BadDoesNotRespawn(t *core.T) {
    fixture := newRunningServiceFixtureWithPolicy(t, RestartNever)
    fixture.process.info = exitedInfo(7)
    close(fixture.process.done)
    core.Eventually(t, func() bool {
        return fixture.serviceSnapshot(t, "api").State == StateFailed
    })
    core.AssertEqual(t, 1, len(fixture.runtime.starts))
}

func TestService_Stop_UglyCancelsPendingRestart(t *core.T) {
    fixture := newPendingRestartFixture(t)
    core.AssertTrue(t, fixture.service.Stop("api").OK)
    fixture.clock.releaseNextRestart()
    core.AssertEqual(t, 1, len(fixture.runtime.starts))
}

func TestService_OnShutdown_GoodStopsEveryRunningService(t *core.T) {
    fixture := newTwoServiceFixture(t)
    result := fixture.service.OnShutdown(core.Background())
    core.AssertTrue(t, result.OK)
    core.AssertEqual(t, 1, fixture.processA.shutdownCalls)
    core.AssertEqual(t, 1, fixture.processB.shutdownCalls)
    core.AssertFalse(t, fixture.service.Start("api").OK)
}
```

- [ ] **Step 2: Write failing bounded event and audit tests**

```go
func TestService_StartAudit_UglyContainsNoExecutionBytes(t *core.T) {
    recorder := installAuditRecorder(t)
    fixture := newStartedServiceFixture(t)
    fixture.definition.Command = "SECRET-COMMAND"
    fixture.definition.Arguments = []string{"SECRET-ARG"}
    _ = fixture.service.Start("api")

    payload := core.Sprintf("%v", recorder.snapshot())
    core.AssertNotContains(t, payload, "SECRET-COMMAND")
    core.AssertNotContains(t, payload, "SECRET-ARG")
    core.AssertContains(t, payload, audit.EventServiceStartRequested)
}

func TestServiceEvent_GoodCarriesOnlyInvalidationMetadata(t *core.T) {
    fixture := newStartedServiceFixture(t)
    events := subscribeServiceEvents(t, fixture.core)
    _ = fixture.service.Start("api")
    core.RequireTrue(t, len(events) > 0)
    serialised := core.JSONMarshal(events[0])
    core.AssertNotContains(t, string(serialised.Value.([]byte)), "command")
}
```

- [ ] **Step 3: Run focused tests and confirm the red state**

Run:

```bash
go test ./go/pkg/services -run 'TestService_(Restart|Stop_Ugly|OnShutdown|StartAudit|Event)' -count=1
```

Expected: failures because restart scheduling, events, audit, and shutdown are
not implemented.

- [ ] **Step 4: Implement generation-bound exit reconciliation and restart budgets**

The observer captures `generation` and `ProcessHandle`. After `Wait`, it reads
`Info`, then updates only when both still match. It computes:

```go
shouldRestart :=
    record.snapshot.Desired &&
    !service.shuttingDown &&
    (definition.RestartPolicy == RestartAlways ||
        (definition.RestartPolicy == RestartOnFailure && info.ExitCode != 0))
```

Use bounded exponential backoff:

```go
delay := limits.RestartBaseDelay << min(attempt, limits.RestartExponentCap)
if delay > limits.RestartMaxDelay {
    delay = limits.RestartMaxDelay
}
```

Retain only restart timestamps inside `Limits.RestartWindow`. When
`Limits.RestartLimit` is exhausted, clear desired state and publish
`ErrorRestartBudgetExhausted`. A cancellation context and generation check
must make Stop, manual Restart, or shutdown invalidate the timer.

- [ ] **Step 5: Implement serialised Restart and bounded parallel shutdown**

`Restart` owns one operation token, clears desired state, shuts down the
current process, then starts a new generation with desired state true. No
public Stop/Start call may interleave.

`OnShutdown` sets `shuttingDown` once, cancels restart work, copies running
process handles while locked, and calls each `Shutdown` in its own goroutine.
Collect one result per process and stop waiting when the supplied context is
done. Join failures with service IDs; never skip another handle because one
failed.

- [ ] **Step 6: Add typed events and audit constants**

Add constants beside the existing process lifecycle constants:

```go
const (
    EventServiceStartRequested  = "service.start.requested"
    EventServiceStartSucceeded  = "service.start.succeeded"
    EventServiceStartFailed     = "service.start.failed"
    EventServiceStopRequested   = "service.stop.requested"
    EventServiceStopSucceeded   = "service.stop.succeeded"
    EventServiceStopFailed      = "service.stop.failed"
    EventServiceRestartRequested = "service.restart.requested"
    EventServiceRestartSucceeded = "service.restart.succeeded"
    EventServiceRestartFailed    = "service.restart.failed"
    EventServiceDefinitionChanged = "service.definition.changed"
)
```

`events.go` provides:

```go
func Subscribe(c *core.Core, fn func(*core.Core, Event))
func (s *Service) fireEvent(operation string, previous, next Snapshot, code ErrorCode)
func (s *Service) auditRequested(name, id string)
func (s *Service) auditSucceeded(name, id, processID string)
func (s *Service) auditFailed(name, id string, result core.Result)
```

Audit Meta allows only `service_id`, `process_id`, `restart_policy`,
`error_code`, and `error_scope`.

- [ ] **Step 7: Run package tests and race checks**

Run:

```bash
go test ./go/pkg/services ./go/pkg/audit -count=1
go test -race ./go/pkg/services -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit supervision and observability**

```bash
git add go/pkg/services/lifecycle.go go/pkg/services/events.go \
  go/pkg/services/restart_test.go go/pkg/services/service_test.go \
  go/pkg/services/audit_test.go go/pkg/audit/types.go
git commit -m "feat(services): supervise managed service lifecycles"
```

---

### Task 5: Register the manager and expose it safely through Wails

**Files:**
- Modify: `go/pkg/services/service.go`
- Modify: `go/pkg/services/wails.go`
- Modify: `go/pkg/services/wails_test.go`
- Modify: `go/pkg/services/wails_example_test.go`
- Modify: `go/cmd/lthn/app.go`
- Modify: `go/pkg/desktop/desktop.go`
- Create: `go/pkg/desktop/services_events.go`
- Create: `go/pkg/desktop/services_events_test.go`

**Interfaces:**
- Consumes: registered Core names `process`, `io`, and `services`; `services.Subscribe`.
- Produces: free `services.Register`, built-in `serve` definition, Wails `Lifecycle` methods, and `lthn:services:changed`.

- [ ] **Step 1: Write failing registration and Wails delegation tests**

```go
func TestRegister_GoodResolvesNamedProcessAndMedium(t *core.T) {
    medium := coreio.NewMemoryMedium()
    c := core.New(
        core.WithName("process", coreprocess.NewService(coreprocess.Options{})),
        core.WithName("io", func(c *core.Core) core.Result {
            return core.Ok(&coreio.Service{
                ServiceRuntime: core.NewServiceRuntime(c, coreio.IOConfig{}),
                Medium:         medium,
            })
        }),
        core.WithName("services", Register),
    )
    core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
    managed, ok := core.ServiceFor[*Service](c, "services")
    core.AssertTrue(t, ok)
    core.AssertNotNil(t, managed)
    core.AssertEqual(t, 0, len(managed.processStartsForTest()))
}

func TestWailsService_Start_GoodDelegatesToManager(t *core.T) {
    manager := newStartedServiceFixture(t).service
    wails := NewWailsService(manager)
    result := wails.Start("api")
    core.AssertTrue(t, result.OK)
    core.AssertEqual(t, StateRunning, result.Value.(Snapshot).State)
}
```

- [ ] **Step 2: Write the failing desktop event-forwarding test**

```go
func TestRegisterServicesEvents_Good(t *core.T) {
    c := core.New()
    emitted := []guievents.TaskEmit{}
    c.Action("events.emit", captureEmits(&emitted))
    registerServicesEvents(c)

    event := services.Event{
        ID: "api", Operation: "start",
        Previous: services.StateStopped, State: services.StateRunning,
        Desired: true, ProcessID: "proc-1",
        At: "2026-07-27T12:00:00Z",
    }
    core.AssertTrue(t, c.ACTION(event).OK)
    core.AssertEqual(t, "lthn:services:changed", emitted[0].Name)
    payload := core.JSONMarshal(emitted[0].Data)
    core.AssertNotContains(t, string(payload.Value.([]byte)), "command")
}
```

- [ ] **Step 3: Run focused tests and confirm failure**

Run:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn \
  -run 'Test(Register|WailsService|RegisterServicesEvents)' -count=1
```

Expected: compile/test failures at the new seams.

- [ ] **Step 4: Implement free registration and the built-in definition**

`Register(c)` retrieves:

```go
processService, processOK := core.ServiceFor[*coreprocess.Service](c, "process")
ioService, ioOK := core.ServiceFor[*coreio.Service](c, "io")
```

Return a typed unavailable failure when either service or
`ioService.Medium` is missing. Construct the catalogue at
`desktop/services/catalogue.json`. The first built-in is:

```go
Definition{
    ID: "serve", DisplayName: "Lethean Desktop API",
    Description: "OpenAI-compatible local Lethean API.",
    Kind: KindService, Command: core.Args()[0],
    Arguments: []string{"serve"}, RestartPolicy: RestartNever,
    GracePeriodMillis: 5_000, Owner: "lethean",
}
```

Reject an empty executable. Do not add the tray host as a managed child.

- [ ] **Step 5: Register after `process` and `io`, preserving shutdown order**

In `newAppCore`, add:

```go
core.WithName("services", services.Register),
```

after the existing `io` registration and before consumers that may register
optional definitions. Because Core shuts services down in reverse registration
order, this placement keeps `services.Service.OnShutdown` ahead of the named
`process.Service.OnShutdown`.

- [ ] **Step 6: Convert Wails to a thin registered-manager wrapper**

```go
type WailsService struct {
    manager *Service
}

func NewWailsService(manager *Service) *WailsService {
    return &WailsService{manager: manager}
}

func (s *WailsService) Catalogue() core.Result { return s.manager.Catalogue() }
func (s *WailsService) Get(id string) core.Result { return s.manager.Get(id) }
func (s *WailsService) Start(id string) core.Result { return s.manager.Start(id) }
func (s *WailsService) Stop(id string) core.Result { return s.manager.Stop(id) }
func (s *WailsService) Restart(id string) core.Result { return s.manager.Restart(id) }
func (s *WailsService) Output(request OutputRequest) core.Result {
    return s.manager.Output(request)
}
func (s *WailsService) SetPolicy(override PolicyOverride) core.Result {
    return s.manager.SetPolicy(override)
}
```

Retain `ServiceName() == "Lifecycle"` and explicit native compatibility
methods as `NativeRegistry`, `InstallNative`, `UninstallNative`,
`StartNative`, `StopNative`, `RestartNative`, and `StatusNative`. Remove no
verified packaging caller; update examples and tests to the explicit names.

- [ ] **Step 7: Bind the registered manager and forward events**

Resolve the manager in `desktop.Service.Run` with:

```go
servicesSvc, ok := core.ServiceFor[*lthnservices.Service](s.opts.Core, "services")
if !ok || servicesSvc == nil {
    return core.Fail(core.E("desktop.Service.Run", "services manager unavailable", nil))
}
```

Bind `lthnservices.NewWailsService(servicesSvc)` and call
`registerServicesEvents(s.opts.Core)` beside `registerFilesEvents`.

- [ ] **Step 8: Run focused Go gates**

Run:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn
```

Expected: PASS. No process should be left running after tests.

- [ ] **Step 9: Commit Core and Wails integration**

```bash
git add go/pkg/services/service.go go/pkg/services/wails.go \
  go/pkg/services/wails_test.go go/pkg/services/wails_example_test.go \
  go/cmd/lthn/app.go go/pkg/desktop/desktop.go \
  go/pkg/desktop/services_events.go go/pkg/desktop/services_events_test.go
git commit -m "feat(desktop): expose managed services through Wails"
```

---

### Task 6: Add the defensive Angular Services bridge

**Files:**
- Create: `frontend-ng/src/app/desktop/desktop-services-bridge.service.ts`
- Create: `frontend-ng/src/app/desktop/desktop-services-bridge.service.spec.ts`
- Create: `frontend-ng/src/app/desktop/apps/control/control-services.models.ts`

**Interfaces:**
- Consumes: Wails `services.WailsService` methods and `ConnectionManagerService.offline`.
- Produces:

```ts
export type DesktopServiceKind = 'service' | 'app' | 'process';
export type DesktopServiceState =
  | 'stopped' | 'starting' | 'running' | 'stopping' | 'exited' | 'failed';
export type DesktopRestartPolicy = 'never' | 'on-failure' | 'always';

export type DesktopServiceErrorCode =
  | 'services_unavailable'
  | 'catalogue_invalid'
  | 'definition_not_found'
  | 'definition_invalid'
  | 'definition_conflict'
  | 'operation_in_progress'
  | 'working_directory_unsupported'
  | 'running_limit_reached'
  | 'process_start_failed'
  | 'process_lookup_failed'
  | 'process_stop_failed'
  | 'restart_budget_exhausted'
  | 'shutdown_incomplete';

export interface DesktopServiceDefinition {
  readonly id: string;
  readonly displayName: string;
  readonly description: string;
  readonly kind: DesktopServiceKind;
  readonly restartPolicy: DesktopRestartPolicy;
  readonly gracePeriodMillis: number;
  readonly owner: string;
}

export interface DesktopServiceFailure {
  readonly code: DesktopServiceErrorCode;
  readonly message: string;
}

export interface DesktopServiceSnapshot {
  readonly definition: DesktopServiceDefinition;
  readonly state: DesktopServiceState;
  readonly desired: boolean;
  readonly processId: string;
  readonly pid: number;
  readonly startedAt: string;
  readonly stoppedAt: string;
  readonly exitCode: number;
  readonly restartCount: number;
  readonly lastError: DesktopServiceFailure | null;
}

export interface DesktopServiceCatalogue {
  readonly services: readonly DesktopServiceSnapshot[];
  readonly refreshedAt: string;
}

export interface DesktopServiceOutput {
  readonly id: string;
  readonly processId: string;
  readonly generation: number;
  readonly output: string;
  readonly truncated: boolean;
  readonly observedAt: string;
}

export interface DesktopServicesChangedEvent {
  readonly id: string;
  readonly operation: string;
  readonly previous: DesktopServiceState;
  readonly state: DesktopServiceState;
  readonly desired: boolean;
  readonly processId: string;
  readonly errorCode: DesktopServiceErrorCode | '';
  readonly at: string;
}

export type ControlServiceIntent =
  | { readonly kind: 'start'; readonly id: string }
  | { readonly kind: 'stop'; readonly id: string }
  | { readonly kind: 'restart'; readonly id: string }
  | { readonly kind: 'output'; readonly id: string };
```

- [ ] **Step 1: Write failing bridge wire and parser tests**

```ts
it('reads and parses the complete services catalogue', async () => {
  surface.call.mockResolvedValue(catalogueWireFixture());
  await expect(service.catalogue()).resolves.toEqual(catalogueWireFixture());
  expect(surface.call).toHaveBeenCalledWith(
    'dappco.re/lthn/desktop/pkg/services.WailsService.Catalogue',
  );
});

it('sends only a known service id for lifecycle mutations', async () => {
  surface.call.mockResolvedValue(snapshotWireFixture());
  await service.start('serve');
  expect(surface.call).toHaveBeenCalledWith(
    'dappco.re/lthn/desktop/pkg/services.WailsService.Start',
    ['serve'],
  );
});

it('rejects execution-bearing or malformed responses', async () => {
  for (const payload of [
    { ...catalogueWireFixture(), command: '/usr/bin/tool' },
    { ...catalogueWireFixture(), arguments: ['--token', 'secret'] },
    { ...catalogueWireFixture(), environment: ['TOKEN=secret'] },
    { ...catalogueWireFixture(), services: [{ ...snapshotWireFixture(), pid: -1 }] },
  ]) {
    surface.call.mockResolvedValue(payload);
    await expect(service.catalogue()).rejects.toThrow('invalid Services response');
  }
});

it('installs no event listener and calls no Wails method offline', async () => {
  offline.set(true);
  expect(service.onChanged(vi.fn())).toEqual(expect.any(Function));
  await expect(service.catalogue()).rejects.toThrow('offline demo mode');
  expect(events.on).not.toHaveBeenCalled();
  expect(surface.call).not.toHaveBeenCalled();
});
```

- [ ] **Step 2: Run the focused spec and confirm failure**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts
```

Expected: test discovery/compile failure because the bridge is absent.

- [ ] **Step 3: Implement method constants, offline guard, parsers, and event source**

```ts
const SERVICE = 'dappco.re/lthn/desktop/pkg/services.WailsService';

export const SERVICES_METHODS = {
  catalogue: `${SERVICE}.Catalogue`,
  get: `${SERVICE}.Get`,
  start: `${SERVICE}.Start`,
  stop: `${SERVICE}.Stop`,
  restart: `${SERVICE}.Restart`,
  output: `${SERVICE}.Output`,
  setPolicy: `${SERVICE}.SetPolicy`,
} as const;

const SERVICES_CHANGED_EVENT = 'lthn:services:changed';
```

Implement `catalogue`, `get`, `start`, `stop`, `restart`, `output`,
`setPolicy`, and `onChanged`. Requests validate the ID locally with the same
closed identifier shape. Parsers require every field and closed enum; PID,
exit code, restart count, generation, output limit, and timestamps are
bounded. A recursive guard rejects keys matching `command`, `arguments`,
`environment`, `workingDirectory`, `root`, `provider`, `credentials`, or
`token`.

- [ ] **Step 4: Add deterministic typed demo data**

In `control-services.models.ts`, export fresh factories:

```ts
export function createDemoServiceCatalogue(): DesktopServiceCatalogue {
  return {
    refreshedAt: 'demo',
    services: [
      demoService('api', 'Lethean API', 'service', 'running', 4821),
      demoService('runner', 'Local model runner', 'service', 'stopped', 0),
      demoService('indexer', 'Workspace indexer', 'process', 'failed', 0),
    ],
  };
}
```

Every demo display name or status area must be paired with `Lethean demo
fixture`; factories return new arrays/objects so windows do not share mutable
demo state.

- [ ] **Step 5: Run bridge tests**

Run:

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts
```

Expected: PASS.

- [ ] **Step 6: Commit the Angular bridge**

```bash
git add src/app/desktop/desktop-services-bridge.service.ts \
  src/app/desktop/desktop-services-bridge.service.spec.ts \
  src/app/desktop/apps/control/control-services.models.ts
git commit -m "feat(frontend): add managed services bridge"
```

---

### Task 7: Turn Control's Daemons tab into the Services manager

**Files:**
- Create: `frontend-ng/src/app/desktop/apps/control/control-services.view.ts`
- Create: `frontend-ng/src/app/desktop/apps/control/control-services.view.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-system.view.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-secondary-views.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control.app.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control.app.spec.ts`

**Interfaces:**
- Consumes: `DesktopDataResource<DesktopServiceCatalogue>`, `DesktopServicesBridgeService`, `DesktopServiceOutput`, and `ControlServiceIntent`.
- Produces: working manual Start/Stop/Restart/Output UI under the stable `daemons` tab value.

- [ ] **Step 1: Write failing presenter tests**

```ts
it('renders manual service controls from a demo resource', async () => {
  const fixture = createServicesViewFixture(
    createDemoResource(createDemoServiceCatalogue(), 'Lethean demo fixture'),
  );
  await fixture.whenStable();
  const element = fixture.nativeElement as HTMLElement;
  expect(element.textContent).toContain('Services');
  expect(element.textContent).toContain('Starts manually');
  expect(element.querySelector('[data-service-id="api"] [data-action="stop"]')).not.toBeNull();
  expect(element.querySelector('[data-service-id="runner"] [data-action="start"]')).not.toBeNull();
});

it('emits a typed restart intent and disables duplicate actions while pending', async () => {
  const fixture = createServicesViewFixture(liveResourceFixture(), ['api']);
  const emitted = vi.fn();
  fixture.componentInstance.action.subscribe(emitted);
  await fixture.whenStable();
  const restart = fixture.nativeElement.querySelector(
    '[data-service-id="api"] [data-action="restart"]',
  ) as HTMLButtonElement;
  expect(restart.disabled).toBe(true);
});

it('shows stale data and bounded output without persisting it', async () => {
  const fixture = createServicesViewFixture(staleResourceFixture(), [], outputFixture());
  await fixture.whenStable();
  expect(fixture.nativeElement.textContent).toContain('Live data stale');
  expect(fixture.nativeElement.querySelector('pre')?.textContent).toContain('ready');
});
```

- [ ] **Step 2: Write failing Control container lifecycle tests**

```ts
it('uses isolated demo service actions without Wails calls', async () => {
  const fixture = await createControl({ offline: true, systab: 'daemons' });
  clickServiceAction(fixture, 'runner', 'start');
  await fixture.whenStable();
  expect(servicesBridge.start).not.toHaveBeenCalled();
  expect(serviceRow(fixture, 'runner').textContent).toContain('Running');
});

it('loads live services, performs Start, then refreshes', async () => {
  servicesBridge.catalogue
    .mockResolvedValueOnce(stoppedCatalogueFixture())
    .mockResolvedValueOnce(runningCatalogueFixture());
  const fixture = await createControl({ offline: false, systab: 'daemons' });
  clickServiceAction(fixture, 'serve', 'start');
  await fixture.whenStable();
  expect(servicesBridge.start).toHaveBeenCalledWith('serve');
  expect(servicesBridge.catalogue).toHaveBeenCalledTimes(2);
});

it('retains stale services after a failed refresh and tears down events', async () => {
  const off = vi.fn();
  servicesBridge.onChanged.mockReturnValue(off);
  const fixture = await createControl({ offline: false, systab: 'daemons' });
  servicesBridge.catalogue.mockRejectedValueOnce(new Error('offline'));
  emitServicesChanged();
  await fixture.whenStable();
  expect(fixture.nativeElement.textContent).toContain('Live data stale');
  fixture.destroy();
  expect(off).toHaveBeenCalledOnce();
});
```

- [ ] **Step 3: Run focused Control specs and confirm failure**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/control/control-services.view.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts
```

Expected: compile/test failures at the new component and container seams.

- [ ] **Step 4: Implement the `OnPush` Services presenter**

`ControlServicesView` receives:

```ts
readonly resource = input.required<DesktopDataResource<DesktopServiceCatalogue>>();
readonly pendingIds = input<readonly string[]>([]);
readonly outputView = input<DesktopServiceOutput | null>(null);
readonly action = output<ControlServiceIntent>();
readonly retry = output<void>();
```

Render `DesktopDataStatusView`, semantic service rows, stable
`data-service-id` and `data-action` hooks, action availability by state,
pending labels, an empty state, and an explicit output `<pre>` only when
requested. Include calm copy:

```text
Services start manually and stop when Lethean Desktop quits.
```

Never render command, arguments, working-directory, or environment fields.

- [ ] **Step 5: Integrate the presenter without changing route state**

In `ControlSystemView`:

- import `ControlServicesView`;
- accept `services`, `pendingServiceIds`, and `serviceOutput` inputs;
- emit `serviceAction` and `servicesRetry`;
- render the component inside `@case ('daemons')`; and
- change only the visible tab label from `Daemons` to `Services`.

Keep `ControlSystemTab = 'overview' | 'processes' | 'daemons'`.

- [ ] **Step 6: Implement Control's demo/live resource lifecycle**

`ControlApp` implements `OnDestroy` and owns:

```ts
readonly servicesResource =
  signal<DesktopDataResource<DesktopServiceCatalogue>>(
    createDemoResource(createDemoServiceCatalogue(), SERVICES_DEMO_SOURCE),
  );
readonly pendingServiceIds = signal<readonly string[]>([]);
readonly serviceOutput = signal<DesktopServiceOutput | null>(null);
```

Connected initialisation replaces this with `createConnectedResource`, installs
one event subscription, and calls guarded `refreshServices`. Use the shared
resource transitions:

```ts
beginDesktopDataRefresh(resource, Date.now(), SERVICES_STALE_AFTER_MS)
resolveDesktopData(resource, catalogue, 'live', SERVICES_LIVE_SOURCE, Date.now())
rejectDesktopData(resource, SERVICES_UNAVAILABLE)
```

`handleServiceAction`:

- mutates only a fresh demo catalogue when mode is demo;
- records the ID in `pendingServiceIds`;
- calls exactly one bridge mutation in connected mode;
- refreshes after successful Start/Stop/Restart;
- loads output only after the explicit output intent;
- maps failures to stale/unavailable without deleting the prior catalogue;
- removes pending state in `finally`; and
- ignores late results after destroy.

- [ ] **Step 7: Run focused Angular tests**

Run:

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts \
  --include=src/app/desktop/apps/control/control-services.view.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts
```

Expected: PASS.

- [ ] **Step 8: Commit the working Services interface**

```bash
git add src/app/desktop/apps/control/control-services.view.ts \
  src/app/desktop/apps/control/control-services.view.spec.ts \
  src/app/desktop/apps/control/control-system.view.ts \
  src/app/desktop/apps/control/control-secondary-views.spec.ts \
  src/app/desktop/apps/control.app.ts \
  src/app/desktop/apps/control.app.spec.ts
git commit -m "feat(frontend): add Control services manager"
```

---

### Task 8: Verify, document, and close the completed slice

**Files:**
- Modify: `TODO.md`
- Modify: `AGENTS.md`
- Modify only if verification reveals a real defect: files changed in Tasks 1–7.

**Interfaces:**
- Consumes: all prior task outputs.
- Produces: verified developer contract and a clean feature handoff.

- [ ] **Step 1: Run formatting and focused source checks**

Run:

```bash
gofmt -w go/pkg/services go/pkg/desktop/services_events.go \
  go/pkg/desktop/services_events_test.go
gofmt -l go/pkg/services go/pkg/desktop/services_events.go \
  go/pkg/desktop/services_events_test.go
git diff --check
```

Expected: `gofmt -l` and `git diff --check` produce no output for changed
files. Do not stage `go.work.sum` or `.playwright-mcp/`.

- [ ] **Step 2: Run focused Go verification**

Run:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/services -count=1
go vet ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn
```

Expected: PASS. If `pkg/desktop` reports `127.0.0.1:9099 address already in
use`, close the development app and rerun; do not change tests to hide the
environmental collision.

- [ ] **Step 3: Run focused and aggregate frontend verification**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts \
  --include=src/app/desktop/apps/control/control-services.view.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts
npm run build
cd ..
go tool wails3 task verify:frontend
```

Expected: all focused specs, the Angular production build, embedded-output
verification, and frontend confidence gate pass.

- [ ] **Step 4: Run the wider Go confidence gate**

Run:

```bash
go tool wails3 task test:go
```

Expected: PASS. Record unrelated pre-existing long security sweep failures
separately rather than changing unrelated packages.

- [ ] **Step 5: Update verified project documentation**

In `TODO.md`, change the Control item to:

```markdown
- [x] Add a typed daemon/service registry with lifecycle state, PID, project
      ownership, bounded restart policy, and start/stop/restart actions.
- [ ] Add provider health/readiness probes and per-service CPU/memory
      telemetry; process liveness is not readiness.
```

In `AGENTS.md`, add under Go composition:

```markdown
- `go/pkg/services/` — manual-by-default background service catalogue and
  lifecycle manager. It persists definitions through the registered
  application `io.Medium` and delegates every runtime operation to the named
  `go-process.Service`. Angular uses the `Lifecycle` Wails wrapper; native
  launchd/systemd installation remains an explicit separate compatibility
  path.
```

Add focused commands:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn -count=1
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts \
  --include=src/app/desktop/apps/control/control-services.view.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts
```

- [ ] **Step 6: Inspect exact scope and commit documentation**

Run:

```bash
git status --short
git diff --stat
git diff -- TODO.md AGENTS.md
git diff --check
```

Expected: the only unrelated entries remain the pre-existing `go.work.sum`
modification and `.playwright-mcp/`.

Commit:

```bash
git add TODO.md AGENTS.md
git commit -m "docs(services): document managed lifecycle"
```

- [ ] **Step 7: Final verification and handoff**

Run:

```bash
git status --short --branch
git log --oneline -10
```

Report:

- exact commits created;
- exact test/build gates and results;
- that nothing auto-starts;
- that services stop on explicit Desktop shutdown;
- that catalogue persistence uses `io.Medium`;
- that runtime operations use the named `go-process.Service`;
- deferred crash adoption/native-host and health telemetry;
- untouched user-owned `go.work.sum` and `.playwright-mcp/`.
