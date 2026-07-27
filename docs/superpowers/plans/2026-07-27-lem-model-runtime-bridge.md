<!-- SPDX-License-Identifier: EUPL-1.2 -->

# LEM Model Runtime Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the optional `lem serve` sidecar a manual Lethean Desktop managed service, with a renderer-safe model-runtime bridge shared by Control and Telemetry.

**Architecture:** `pkg/services` owns the child process while a new `pkg/modelruntime` service owns readiness, credentials, model selection, state, history, and safe Wails DTOs. A new Medium-backed catalogue in `pkg/models` converts trusted LEM model directories into opaque IDs, and Angular shares one strictly parsed runtime resource between every consumer.

**Tech Stack:** Go 1.26, CoreGO `core.Core`, `dappco.re/go/io.Medium`, `dappco.re/go/process`, Wails 3 alpha, Angular 22 standalone components, signals, Vitest, npm, Task.

## Global Constraints

- `frontend-ng/` remains the only product frontend; do not add SSR, hydration, or another frontend framework.
- `lthn serve` remains the Desktop/CoreGO API and must work when the frontend or LEM sidecar is absent.
- `inference` is manual-by-default and must never start from registration, startup, snapshot reads, events, Control, Telemetry, or tray rendering.
- The trusted sidecar arguments are exactly `serve --addr 127.0.0.1:36911 --shutdown-timeout 10s`; omit `--model` and CORS flags.
- Every model and credential file operation flows through a registered `io.Medium`; no raw `os`, `path/filepath`, `syscall`, or Core-Fs fallback is allowed in the new path.
- Renderer contracts never include executable paths, model paths, arguments, environment, working directories, arbitrary URLs, or credentials.
- The loopback LEM endpoint is fixed at `http://127.0.0.1:36911`; redirects and oversized or non-JSON responses fail closed.
- LEM owns `~/Lethean/lem/admin.token`; Desktop reads `lem/admin.token` through its trusted root Medium and never invokes `--print-admin-token`.
- Current single-model LEM has reload but no mounted unload or metrics route. Desktop implements `Unload` by restarting the managed sidecar model-less and leaves unsupported metrics absent.
- Runtime history is in-memory, sampled every five seconds while the explicitly desired process is running, and capped at 720 entries.
- Offline demo mode makes zero Wails calls and zero Wails event subscriptions.
- Use British English and EUPL-1.2 headers.
- No test starts the real `lem`, binds port 36911, or loads model weights.
- Preserve the user's existing `go.work.sum` modification and `.playwright-mcp/` directory outside all commits.

---

### Task 1: Register the manual LEM managed service

**Files:**
- Modify: `go/pkg/services/service.go`
- Modify: `go/pkg/services/registration_test.go`
- Modify: `go/pkg/services/service_example_test.go`

**Interfaces:**
- Consumes: `core.Args()`, `core.PathDir`, `core.PathJoin`, `services.Definition`.
- Produces: built-in service ID `inference`, resolved only from the trusted sibling of the running `lthn` executable.

- [ ] **Step 1: Write the failing registration contract**

Add assertions to `TestRegister_GoodResolvesNamedProcessAndMediumWithoutStarting`:

```go
core.RequireTrue(t, len(snapshots) == 2)
byID := map[string]Snapshot{}
for _, snapshot := range snapshots {
    byID[snapshot.Definition.ID] = snapshot
}
core.AssertEqual(t, StateStopped, byID["inference"].State)
core.AssertFalse(t, byID["inference"].Desired)
core.AssertEqual(t, 0, len(processes.List()))
```

Add an internal-package test which calls `service.definition("inference")` and proves:

```go
core.AssertEqual(t, []string{
    "serve", "--addr", "127.0.0.1:36911", "--shutdown-timeout", "10s",
}, definition.Arguments)
core.AssertEqual(t, RestartNever, definition.RestartPolicy)
core.AssertEqual(t, int64(15_000), definition.GracePeriodMillis)
core.AssertNotContains(t, definition.Arguments, "--model")
core.AssertNotContains(t, definition.Arguments, "--cors")
```

- [ ] **Step 2: Run the focused test and observe the missing definition**

Run:

```bash
go test ./go/pkg/services -run 'TestRegister_Good|TestRegister_Inference' -count=1
```

Expected: FAIL because only `serve` is registered.

- [ ] **Step 3: Add the trusted sibling resolver and definition**

In `go/pkg/services/service.go`, keep executable resolution private:

```go
func inferenceExecutable(lthnExecutable string) string {
    name := "lem"
    if runtime.GOOS == "windows" {
        name = "lem.exe"
    }
    return core.PathJoin(core.PathDir(lthnExecutable), name)
}
```

Append this built-in beside `serve`:

```go
{
    ID:                "inference",
    DisplayName:       "LEM inference runtime",
    Description:       "Optional local model runtime, started only on request.",
    Kind:              KindService,
    Command:           inferenceExecutable(args[0]),
    Arguments:         []string{"serve", "--addr", "127.0.0.1:36911", "--shutdown-timeout", "10s"},
    RestartPolicy:     RestartNever,
    GracePeriodMillis: 15_000,
    Owner:             "lethean",
},
```

Do not stat the executable or add `exec.LookPath`; `go-process` remains the sole execution boundary.

- [ ] **Step 4: Run the package tests and race test**

Run:

```bash
go test ./go/pkg/services -count=1
go test -race ./go/pkg/services -count=1
```

Expected: PASS, with no process start during registration.

- [ ] **Step 5: Commit**

```bash
git add go/pkg/services/service.go go/pkg/services/registration_test.go go/pkg/services/service_example_test.go
git commit -m "feat(services): register manual LEM runtime"
```

---

### Task 2: Add a Medium-backed, path-safe model catalogue

**Files:**
- Create: `go/pkg/models/catalogue.go`
- Create: `go/pkg/models/catalogue_test.go`
- Create: `go/pkg/models/catalogue_example_test.go`

**Interfaces:**
- Consumes: `io.Medium`, a trusted native root supplied by composition.
- Produces:

```go
type CatalogueEntry struct {
    ID          string `json:"id"`
    DisplayName string `json:"displayName"`
    Format      string `json:"format"`
    SizeBytes   int64  `json:"sizeBytes"`
    Loadable    bool   `json:"loadable"`
}

type Reference struct {
    ID           string
    DisplayName  string
    RelativePath string
    NativePath   string
    Format       string
}

func NewCatalogue(medium io.Medium, relativeRoot, nativeRoot string) *Catalogue
func (catalogue *Catalogue) List() core.Result
func (catalogue *Catalogue) Resolve(id string) core.Result
```

- [ ] **Step 1: Write Good/Bad/Ugly catalogue tests**

Use `io.NewMemoryMedium()` with `relativeRoot == "lem/models"` and plant:

```go
core.RequireNoError(t, medium.EnsureDir("lem/models/gemma-4-e2b"))
core.RequireNoError(t, medium.Write("lem/models/gemma-4-e2b/.sha256", "digest  weights.safetensors\n"))
core.RequireNoError(t, medium.Write("lem/models/gemma-4-e2b/weights.safetensors", "weights"))
core.RequireNoError(t, medium.EnsureDir("lem/models/unverified"))
```

Prove:

```go
entries := result.Value.([]CatalogueEntry)
core.AssertEqual(t, "gemma-4-e2b", entries[0].DisplayName)
core.AssertTrue(t, entries[0].Loadable)
core.AssertNotContains(t, core.Sprintf("%+v", entries[0]), "/models/")
```

Resolve the opaque ID and assert the trusted internal reference contains `NativePath == core.PathJoin(nativeRoot, "gemma-4-e2b")`. Also prove an unknown ID, traversal-shaped ID, missing Medium, and missing native capability fail before returning a path.

- [ ] **Step 2: Run the tests and observe the missing API**

Run:

```bash
go test ./go/pkg/models -run Catalogue -count=1
```

Expected: FAIL because `Catalogue` is undefined.

- [ ] **Step 3: Implement bounded opaque identity and Medium-only discovery**

`Catalogue.List` must call only `medium.List(relativeRoot)`, `medium.Stat`, and `medium.IsFile`. Include immediate child directories only. A child is loadable only when `<relativeRoot>/<name>/.sha256` is a file.

Generate the renderer ID from a stable hash without exposing the path:

```go
func modelID(name string) string {
    sum := sha256.Sum256([]byte(name))
    return "model-" + hex.EncodeToString(sum[:8])
}
```

Reject names longer than 255 bytes, hidden names, control characters, separators, and `.`/`..`. Sort entries by `DisplayName`. Cap the catalogue at 512 entries. `Resolve` re-lists, matches the opaque ID, requires `Loadable`, and constructs `NativePath` only from the trusted native root plus the validated relative child name.

- [ ] **Step 4: Run tests, vet, and the no-raw-filesystem diagnostic**

Run:

```bash
go test ./go/pkg/models -run Catalogue -count=1
go vet ./go/pkg/models
rg -n '"(os|path/filepath|syscall)"|core\.(Stat|ReadDir|Open|ReadFile|WriteFile)' go/pkg/models/catalogue.go
```

Expected: tests and vet PASS; `rg` prints no matches.

- [ ] **Step 5: Commit**

```bash
git add go/pkg/models/catalogue.go go/pkg/models/catalogue_test.go go/pkg/models/catalogue_example_test.go
git commit -m "feat(models): add Medium-backed runtime catalogue"
```

---

### Task 3: Add the bounded LEM protocol client and credential provider

**Files:**
- Create: `go/pkg/modelruntime/client.go`
- Create: `go/pkg/modelruntime/client_test.go`
- Create: `go/pkg/modelruntime/credential.go`
- Create: `go/pkg/modelruntime/credential_test.go`
- Modify: `go/pkg/lemma/admin.go`
- Modify: `go/pkg/lemma/admin_test.go`

**Interfaces:**
- Consumes: fixed loopback endpoint, `io.Medium`, explicit bearer credentials.
- Produces:

```go
type Client interface {
    Health(core.Context) core.Result
    Status(core.Context, string) core.Result
    Machine(core.Context, string) core.Result
    Reload(core.Context, string, ReloadCommand) core.Result
}

type CredentialProvider interface {
    Credential() core.Result
    Invalidate()
    Clear()
}
```

- [ ] **Step 1: Write HTTP safety tests**

Use `httptest.NewServer` only to exercise a test constructor. Prove:

- health decodes `{"status":"ok","runtime":"go-inference","models":[],"time":1}`;
- status rejects non-JSON content;
- redirect responses are rejected without following;
- a body larger than 1 MiB returns `response_too_large`;
- a delayed response returns `request_timeout`;
- a 401 returns the typed internal sentinel `errUnauthorised`;
- reload sends the bearer header and `confirm_machine` but never returns the request path.

The production constructor test must assert:

```go
client := NewClient()
core.AssertEqual(t, "http://127.0.0.1:36911", client.baseURL)
```

- [ ] **Step 2: Write credential tests**

Use `io.NewMemoryMedium()` and assert:

```go
core.RequireNoError(t, medium.Write("lem/admin.token", "lem_admin_1234567890abcdef\n"))
first := provider.Credential()
second := provider.Credential()
core.AssertEqual(t, first.String(), second.String())
```

Wrap the Medium with a read counter to prove caching; call `Invalidate` and prove one re-read. Reject empty, whitespace-bearing, control-bearing, and greater-than-512-byte tokens. A nil Medium must fail closed without a read.

- [ ] **Step 3: Run the focused tests and observe failure**

Run:

```bash
go test ./go/pkg/modelruntime ./go/pkg/lemma -run 'Client|Credential|Admin' -count=1
```

Expected: FAIL because the new package and explicit client helpers do not exist.

- [ ] **Step 4: Implement the protocol boundary**

Use one `http.Client` configured with:

```go
&http.Client{
    Timeout: 5 * time.Second,
    CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
        return http.ErrUseLastResponse
    },
}
```

Cap every response with `io.LimitReader(resp.Body, maxResponseBytes+1)`, require JSON content for decoded responses, and never include response bodies in public failures.

Extend `lemma.Admin` only where useful for the typed status/reload shapes, but construct it with an explicit token from `pkg/modelruntime`; the new path must never use `AdminConfig.TokenPath` or `loadTokenFromFile`.

- [ ] **Step 5: Run tests and vet**

Run:

```bash
go test ./go/pkg/modelruntime ./go/pkg/lemma -run 'Client|Credential|Admin' -count=1
go vet ./go/pkg/modelruntime ./go/pkg/lemma
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/pkg/modelruntime/client.go go/pkg/modelruntime/client_test.go go/pkg/modelruntime/credential.go go/pkg/modelruntime/credential_test.go go/pkg/lemma/admin.go go/pkg/lemma/admin_test.go
git commit -m "feat(modelruntime): add bounded LEM client"
```

---

### Task 4: Implement model-runtime state, operations, events, and Wails facade

**Files:**
- Create: `go/pkg/modelruntime/types.go`
- Create: `go/pkg/modelruntime/service.go`
- Create: `go/pkg/modelruntime/operations.go`
- Create: `go/pkg/modelruntime/events.go`
- Create: `go/pkg/modelruntime/wails.go`
- Create: `go/pkg/modelruntime/service_test.go`
- Create: `go/pkg/modelruntime/operations_test.go`
- Create: `go/pkg/modelruntime/wails_test.go`
- Create: `go/pkg/modelruntime/modelruntime_example_test.go`

**Interfaces:**
- Consumes:

```go
type Lifecycle interface {
    Get(string) core.Result
    Start(string) core.Result
    Stop(string) core.Result
    Restart(string) core.Result
}

type ModelCatalogue interface {
    List() core.Result
    Resolve(string) core.Result
}
```

- Produces:

```go
func NewService(Options) *Service
func Register(*core.Core) core.Result
func Subscribe(*core.Core, func(*core.Core, Event))

func (service *Service) Snapshot() core.Result
func (service *Service) Start() core.Result
func (service *Service) Load(LoadRequest) core.Result
func (service *Service) Unload() core.Result
func (service *Service) Restart() core.Result
func (service *Service) Stop() core.Result

type WailsService struct{ runtime *Service }
func NewWailsService(*Service) *WailsService
func (*WailsService) ServiceName() string // "ModelRuntime"
```

- [ ] **Step 1: Write state and non-start regression tests**

Define the closed states and safe DTOs in the tests:

```go
var states = []State{
    StateUnavailable, StateStopped, StateStarting, StateModelLess,
    StateLoading, StateReady, StateDegraded, StateFailed, StateStopping,
}
```

With fake lifecycle/client/catalogue/credential/clock/event dependencies, call construction, registration, `Snapshot`, and subscription. Assert `lifecycle.startCalls == 0` and `client.healthCalls == 0` while the managed snapshot is stopped.

- [ ] **Step 2: Write operation-order and failure tests**

Record fake calls and prove:

```go
core.AssertEqual(t, []string{
    "catalogue.resolve", "lifecycle.get", "lifecycle.start",
    "client.health", "credential.read", "client.machine", "client.reload",
    "client.health", "client.status",
}, calls)
```

Also prove:

- invalid/non-loadable IDs fail before start;
- concurrent mutation returns `operation_in_progress`;
- health loss retains the last-good model and sets stale/degraded;
- one unauthorised response invalidates and retries the credential exactly once;
- restart never calls reload;
- unload calls lifecycle restart and returns model-less;
- stop during a load cancels polling before lifecycle stop;
- all failures use the closed safe codes;
- history truncates to the newest 720 entries;
- event JSON contains only `reason`, `state`, and `at`.

- [ ] **Step 3: Run the tests and observe missing implementation**

Run:

```bash
go test ./go/pkg/modelruntime -count=1
```

Expected: FAIL because the service is not implemented.

- [ ] **Step 4: Implement immutable safe types and failure mapping**

Use pointer metrics so unsupported values remain absent:

```go
type MetricsView struct {
    PromptTokensPerSecond *float64 `json:"promptTokensPerSecond,omitempty"`
    DecodeTokensPerSecond *float64 `json:"decodeTokensPerSecond,omitempty"`
    ActiveMemoryBytes     *uint64  `json:"activeMemoryBytes,omitempty"`
    PeakMemoryBytes       *uint64  `json:"peakMemoryBytes,omitempty"`
    KVCacheBytes          *uint64  `json:"kvCacheBytes,omitempty"`
    UptimeSeconds         *int64   `json:"uptimeSeconds,omitempty"`
}
```

Clone all slices before returning them. Cap model rows at 512, history at 720, strings at their declared bounds, and messages at 512 bytes. Never serialise `models.Reference`, credentials, client URLs, service definitions, or upstream response bodies.

- [ ] **Step 5: Implement mutation serialisation and readiness**

Use a non-blocking operation gate:

```go
select {
case service.operation <- struct{}{}:
    defer func() { <-service.operation }()
default:
    return fail(ErrorOperationInProgress, "Another model-runtime operation is already running.")
}
```

Use a cancellable generation context, bounded readiness deadline, and backoff sequence `50ms, 100ms, 200ms, 400ms, 800ms` in tests with injectable `After`. Production readiness is capped at 30 seconds.

`Unload` must use `Lifecycle.Restart("inference")`, then readiness, because the current single-model LEM host does not mount `/v1/admin/models/unload`.

- [ ] **Step 6: Implement sampling, Core invalidations, and shutdown**

Sampler construction must not start a process. When a snapshot observes `Desired && running`, start one five-second ticker loop for that generation. Stop the loop when the generation changes, the process stops, or Core shuts down.

Publish:

```go
type Event struct {
    Reason string `json:"reason"`
    State  State  `json:"state"`
    At     string `json:"at"`
}
```

`OnShutdown` rejects new operations, cancels loops, waits for in-flight work, and calls `CredentialProvider.Clear`. It does not independently stop the child; `pkg/services` remains lifecycle owner.

- [ ] **Step 7: Implement the exact Wails surface**

Expose only:

```go
Snapshot()
Start()
Load(LoadRequest)
Unload()
Restart()
Stop()
```

The Wails wrapper delegates to the Core-owned service and fails closed when nil. Add a reflection test that sorts exported method names and matches the expected set plus Wails lifecycle/name methods.

- [ ] **Step 8: Run focused, race, and vet checks**

Run:

```bash
go test ./go/pkg/modelruntime -count=1
go test -race ./go/pkg/modelruntime -count=1
go vet ./go/pkg/modelruntime
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add go/pkg/modelruntime
git commit -m "feat(modelruntime): manage LEM runtime state"
```

---

### Task 5: Compose trusted Media and bind ModelRuntime into Desktop

**Files:**
- Modify: `go/cmd/lthn/app.go`
- Modify: `go/cmd/lthn/app_test.go`
- Modify: `go/pkg/desktop/desktop.go`
- Create: `go/pkg/desktop/modelruntime_events.go`
- Create: `go/pkg/desktop/modelruntime_events_test.go`
- Modify: `go/pkg/desktop/runtime_events.go`
- Modify: `go/pkg/desktop/desktop_test.go`

**Interfaces:**
- Consumes: named `services` and `lem-io` services.
- Produces: Core service `modelruntime`, Wails binding `ModelRuntime`, native event `lthn:model-runtime:changed`.

- [ ] **Step 1: Write failing composition tests**

Extend the binding catalogue assertion with:

```go
core.AssertContains(t, wailsBindingCatalogue, "modelruntime.NewWailsService(modelRuntimeSvc)")
core.AssertEqual(t, 46, len(wailsBindingCatalogue))
```

Add a Core registration test using temporary `HOME`, call startup, resolve:

```go
runtime, ok := core.ServiceFor[*modelruntime.Service](c, "modelruntime")
core.RequireTrue(t, ok && runtime != nil)
manager := core.MustServiceFor[*services.Service](c, "services")
snapshot := manager.Get("inference").Value.(services.Snapshot)
core.AssertEqual(t, services.StateStopped, snapshot.State)
```

Assert no process exists.

- [ ] **Step 2: Run focused composition tests**

Run:

```bash
go test ./go/cmd/lthn ./go/pkg/desktop -run 'ModelRuntime|WailsBinding|NewAppCore' -count=1
```

Expected: FAIL because composition and forwarding are absent.

- [ ] **Step 3: Register trusted root and model Media**

Resolve `paths.Root()` once in `newAppCore`. Register:

```go
core.WithName("lem-io", io.NewService(io.IOConfig{
    Root: root.Value.(string),
})),
core.WithName("modelruntime", modelruntime.Register),
```

The root value is trusted composition data. `modelruntime.Register` gives the same Medium to the credential provider at `lem/admin.token` and the model catalogue at `lem/models`, while supplying `core.PathJoin(root, "lem", "models")` only as the trusted native execution root. Do not pass any root to Angular. An absent `lem/models` prefix returns `catalogue_unavailable`; browsing must not create it.

- [ ] **Step 4: Bind and forward events**

Resolve `modelRuntimeSvc` alongside `servicesSvc`, bind `modelruntime.NewWailsService(modelRuntimeSvc)`, and forward:

```go
modelruntime.Subscribe(c, func(c *core.Core, event modelruntime.Event) {
    emitCoreEvent(c, "lthn:model-runtime:changed", event)
})
```

Add an event test matching the existing services/files event tests. The emitted payload must not contain `path`, `token`, `command`, `arguments`, or `url`.

- [ ] **Step 5: Keep the legacy Lemma binding only until its consumers migrate**

Keep `gui.Bind(lemma.NewWailsService(lemma.AdminConfig{}))` in this commit because the tray still consumes it. Add `ModelRuntime` beside it, update `wailsBindingCatalogue` to 46, and add a comment that Task 7 removes the renderer-facing legacy binding after the tray migration. `pkg/lemma.Admin` remains an internal typed protocol helper.

- [ ] **Step 6: Run focused tests and dry binding generation**

Run:

```bash
go test ./go/cmd/lthn ./go/pkg/desktop ./go/pkg/modelruntime ./go/pkg/services -count=1
cd go
go tool wails3 generate bindings -ts -dry -f -tags mcp ./pkg/desktop/...
```

Expected: PASS and no Wails warnings.

- [ ] **Step 7: Commit**

```bash
git add go/cmd/lthn/app.go go/cmd/lthn/app_test.go go/pkg/desktop/desktop.go go/pkg/desktop/modelruntime_events.go go/pkg/desktop/modelruntime_events_test.go go/pkg/desktop/runtime_events.go go/pkg/desktop/desktop_test.go
git commit -m "feat(desktop): bind the LEM runtime bridge"
```

---

### Task 6: Add the strict Angular bridge and shared runtime resource

**Files:**
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime.models.ts`
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime-bridge.service.ts`
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime-bridge.service.spec.ts`
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime-resource.service.ts`
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime-resource.service.spec.ts`
- Create: `frontend-ng/src/app/desktop/desktop-model-runtime-demo.data.ts`

**Interfaces:**
- Consumes: exact Wails methods under `dappco.re/lthn/desktop/pkg/modelruntime.WailsService`.
- Produces:

```ts
export type ModelRuntimeState =
  | 'unavailable' | 'stopped' | 'starting' | 'model-less'
  | 'loading' | 'ready' | 'degraded' | 'failed' | 'stopping';

export interface ModelRuntimeSnapshot { /* safe fields from the Go DTO */ }

export class DesktopModelRuntimeBridgeService {
  snapshot(): Promise<ModelRuntimeSnapshot>;
  start(): Promise<ModelRuntimeSnapshot>;
  load(modelId: string): Promise<ModelRuntimeSnapshot>;
  unload(): Promise<ModelRuntimeSnapshot>;
  restart(): Promise<ModelRuntimeSnapshot>;
  stop(): Promise<ModelRuntimeSnapshot>;
  onChanged(handler: (event: ModelRuntimeChangedEvent) => void): () => void;
}

export class DesktopModelRuntimeResource {
  readonly resource: Signal<DesktopDataResource<ModelRuntimeSnapshot>>;
  readonly pending: Signal<ModelRuntimeOperation | null>;
  connect(): () => void;
  refresh(): Promise<void>;
  perform(operation: ModelRuntimeOperation): Promise<void>;
}
```

- [ ] **Step 1: Write strict bridge tests**

Mock `SurfaceBridgeService`, `ConnectionManagerService`, and the Wails event adapter. Prove:

- offline methods reject before calling the bridge;
- offline `onChanged` returns a no-op without subscribing;
- exact method names and `{modelId}` request shape;
- every closed state parses;
- unknown state, NaN/Infinity, more than 512 models, more than 720 samples, invalid timestamps, and oversized strings reject;
- recursive forbidden-field scanning rejects `path`, `model_path`, `command`, `arguments`, `environment`, `workingDirectory`, `endpoint`, `url`, `token`, `secret`, `credential`, and `key`.

- [ ] **Step 2: Run the bridge spec and observe failure**

Run:

```bash
cd frontend-ng
npx ng test --watch=false --include=src/app/desktop/desktop-model-runtime-bridge.service.spec.ts
```

Expected: FAIL because the bridge does not exist.

- [ ] **Step 3: Implement models, exact calls, and defensive parsing**

Use these method constants:

```ts
const SNAPSHOT = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Snapshot';
const START = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Start';
const LOAD = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Load';
const UNLOAD = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Unload';
const RESTART = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Restart';
const STOP = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService.Stop';
```

Call `rejectForbiddenFields(raw)` before parsing. Optional metric fields may be absent but, when present, must be finite and non-negative.

- [ ] **Step 4: Write shared-resource tests**

Use fake timers. Call `connect()` twice and prove one listener and one fallback interval. Trigger five events in one microtask and prove one snapshot call. Reject the next refresh and prove the last value remains with state `stale`. Dispose both consumers and prove listener/timer teardown. Resolve a late promise after disposal and prove the resource does not change.

Demo tests must call `perform({kind: 'load', modelId})` and prove only deterministic in-memory state changes, with no bridge methods invoked.

- [ ] **Step 5: Implement ref-counted reconciliation**

The first live consumer installs one event listener and a 30-second recovery timer. Queue event refreshes with one pending microtask. Use `beginDesktopDataRefresh`, `resolveDesktopData`, and `rejectDesktopData` so last-good data is preserved.

In demo mode initialise one fresh `createDemoModelRuntimeSnapshot()` per root service instance and simulate:

```text
stopped → starting → model-less → loading → ready
ready → model-less
any running state → stopping → stopped
```

- [ ] **Step 6: Run both specs and type checking**

Run:

```bash
npx ng test --watch=false \
  --include=src/app/desktop/desktop-model-runtime-bridge.service.spec.ts \
  --include=src/app/desktop/desktop-model-runtime-resource.service.spec.ts
npx tsc --noEmit -p tsconfig.app.json
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend-ng/src/app/desktop/desktop-model-runtime*
git commit -m "feat(frontend): share the LEM runtime resource"
```

---

### Task 7: Drive Control, Telemetry, and the tray from the shared runtime

**Files:**
- Modify: `frontend-ng/src/app/desktop/apps/control.app.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control.app.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-models.view.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-primary-views.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-view.models.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-view-state.ts`
- Modify: `frontend-ng/src/app/desktop/apps/control/control-view-state.spec.ts`
- Modify: `frontend-ng/src/app/desktop/desktop-live-data.service.ts`
- Modify: `frontend-ng/src/app/desktop/desktop-live-data.service.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/telemetry.app.ts`
- Modify: `frontend-ng/src/app/desktop/apps/telemetry.app.spec.ts`
- Modify: `frontend-ng/src/app/desktop/apps/telemetry/telemetry-view.models.ts`
- Modify: `frontend-ng/src/app/desktop/apps/telemetry/telemetry-view-state.ts`
- Modify: `frontend-ng/src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts`
- Modify: `frontend-ng/src/app/tray-panel/tray-panel.ts`
- Modify: `frontend-ng/src/app/tray-panel/tray-panel.html`
- Modify: `frontend-ng/src/app/tray-panel/tray-panel.spec.ts`
- Modify: `go/pkg/desktop/desktop.go`
- Modify: `go/cmd/lthn/app.go`
- Modify: `go/cmd/lthn/app_test.go`

**Interfaces:**
- Consumes: `DesktopModelRuntimeResource`.
- Produces: explicit Control runtime actions, shared Control/Telemetry truth, path-free tray status.

- [ ] **Step 1: Write presenter and container tests**

Prove Control:

- lists opaque model IDs and display names, never native paths;
- shows Start only while stopped;
- shows Load while model-less/ready;
- shows Unload while ready;
- shows Restart while ready/degraded/failed;
- shows Stop while starting/model-less/loading/ready/degraded/failed;
- disables conflicting actions while `pending()` is non-null;
- routes a selected model ID to `resource.perform({kind: 'load', modelId})`.

Prove Control and Telemetry call `resource.connect()` and receive the same snapshot object. Prove unsupported throughput/memory renders `—`, not `0` or demo numbers.

Prove the tray calls `ModelRuntime.Snapshot`, parses `activeModelId`/safe model display names, and contains no basename/path parsing.

- [ ] **Step 2: Run the affected specs and observe failure**

Run:

```bash
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/control/control-primary-views.spec.ts \
  --include=src/app/desktop/apps/control/control-view-state.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts \
  --include=src/app/tray-panel/tray-panel.spec.ts
```

Expected: FAIL against the fixture-derived runtime UI.

- [ ] **Step 3: Remove the legacy absolute-path model read from Control**

Delete `MODELS_METHOD`, `LocalModelEntry.path`, and `DesktopLiveDataService.models()`. Remove `models` from `ControlDataSection` and from `control()`'s `Promise.allSettled` list. Control's model rows now come only from `ModelRuntimeSnapshot.models`.

- [ ] **Step 4: Extend the Control models presenter**

Change `ControlModelsViewModel` to carry:

```ts
readonly state: ModelRuntimeState;
readonly activeModelId: string;
readonly availableModels: readonly ModelRuntimeModel[];
readonly metrics: readonly ControlMetric[];
readonly chart: ControlChart;
```

Emit typed actions:

```ts
type ControlModelIntent =
  | { readonly kind: 'start' }
  | { readonly kind: 'load'; readonly modelId: string }
  | { readonly kind: 'unload' }
  | { readonly kind: 'restart' }
  | { readonly kind: 'stop' };
```

Use an accessible selection control keyed by opaque ID. Preserve the current tiles, chart, table, filters, and responsive structure; organise the actions without deleting existing UX.

- [ ] **Step 5: Merge shared runtime data into Control and Telemetry**

Control keeps Desktop process/settings/benchmark data in its existing resource and overlays model-runtime data from the shared resource. Telemetry keeps process heap/power data in its existing resource and overlays runtime model/memory/throughput fields from the same shared snapshot.

Do not copy demo throughput into a connected snapshot. Where LEM exposes no metric, use `—` and an empty chart series.

- [ ] **Step 6: Migrate the tray**

Replace `LEMMA_STATUS_METHOD` with `ModelRuntime.Snapshot`. Parse the safe DTO with the same state bounds as the main bridge or reuse an exported pure parser. Derive the display name by matching `activeModelId` to `models`; do not accept `model_path`.

- [ ] **Step 7: Remove the obsolete renderer-facing Lemma binding**

After the tray no longer calls it, remove `gui.Bind(lemma.NewWailsService(lemma.AdminConfig{}))` and its renderer catalogue entry. Keep `pkg/lemma.Admin` for trusted Go-side protocol reuse. Update the exact Wails catalogue pin from 46 back to 45 and run the dry binding generator to prove the intended schema change emits no warning.

- [ ] **Step 8: Run affected and aggregate frontend gates**

Run:

```bash
npx ng test --watch=false \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/control/control-primary-views.spec.ts \
  --include=src/app/desktop/apps/control/control-view-state.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts \
  --include=src/app/tray-panel/tray-panel.spec.ts
npm run test:ci
npm run build
cd ../go
go tool wails3 generate bindings -ts -dry -f -tags mcp ./pkg/desktop/...
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add frontend-ng/src/app/desktop frontend-ng/src/app/tray-panel go/pkg/desktop/desktop.go go/cmd/lthn/app.go go/cmd/lthn/app_test.go
git commit -m "feat(frontend): wire Control and Telemetry to LEM"
```

---

### Task 8: Stage LEM, document the contract, and run completion gates

**Files:**
- Modify: `Taskfile.yml`
- Modify: `build/darwin/Taskfile.yml`
- Modify: `build/linux/Taskfile.yml`
- Modify: `build/windows/Taskfile.yml`
- Modify: `scripts/dev-doctor.mjs`
- Modify: `scripts/dev-doctor.test.mjs`
- Create: `scripts/verify-model-runtime-convergence.test.mjs`
- Modify: `frontend-ng/package.json`
- Modify: `docs/development.md`
- Modify: `AGENTS.md`
- Modify: `TODO.md`

**Interfaces:**
- Consumes: `LTHN_LEM_REPO` or `LTHN_LEM_BIN` at build time.
- Produces: fixed sibling `lem`/`lem.exe` package layout and executable convergence proof.

- [ ] **Step 1: Write the failing convergence test**

The Node test must read Taskfiles and Go source, then assert:

```js
assert.match(serviceSource, /"inference"/);
assert.match(serviceSource, /"serve", "--addr", "127\.0\.0\.1:36911"/);
assert.doesNotMatch(serviceSource, /exec\.LookPath|--model|--cors/);
assert.match(rootTask, /LTHN_LEM_REPO/);
assert.match(darwinTask, /Contents\/MacOS\/lem/);
assert.match(linuxTask, /\blem\b/);
assert.match(windowsTask, /lem\.exe/);
```

Scan modelruntime Wails DTO source and Angular model interfaces for forbidden renderer fields.

- [ ] **Step 2: Run the contract and observe failure**

Run:

```bash
node --test scripts/verify-model-runtime-convergence.test.mjs
```

Expected: FAIL because packaging does not stage LEM.

- [ ] **Step 3: Add sidecar build/staging tasks**

Add:

```yaml
LTHN_LEM_REPO: '{{.LTHN_LEM_REPO | default "/Users/snider/Code/core/go-inference"}}'
```

`build:lem` uses the repository's supported build task, then copies its `bin/lem` into `build/<platform>/bin/lem[.exe]`. `LTHN_LEM_BIN` can supply a prebuilt matching-platform binary in CI. Missing optional source prints one explicit warning and leaves runtime unavailable; it must not add a PATH fallback.

macOS packaging copies the sidecar to `Contents/MacOS/lem`. Linux and Windows package definitions copy it beside the installed `lthn` executable when supplied.

- [ ] **Step 4: Extend doctor and docs**

Doctor reports:

```text
LEM sidecar: ready at bin/lem
```

or:

```text
LEM sidecar: optional runtime unavailable; build it or set LTHN_LEM_BIN
```

Document the manual lifecycle, port 36911, model-less start, model directory `~/Lethean/lem/models`, token boundary, demo mode, and focused checks. In `TODO.md`, record the upstream telemetry endpoint needed for real throughput/KV/request metrics; state that live UI intentionally renders those values unavailable until the endpoint exists.

- [ ] **Step 5: Run convergence and focused verification**

Run:

```bash
node --test scripts/verify-model-runtime-convergence.test.mjs scripts/dev-doctor.test.mjs scripts/verify-frontend-convergence.test.mjs
go test ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn -count=1
go test -race ./go/pkg/services ./go/pkg/modelruntime -count=1
go vet ./go/pkg/services ./go/pkg/models ./go/pkg/modelruntime ./go/pkg/desktop ./go/cmd/lthn
cd frontend-ng
npx ng test --watch=false \
  --include=src/app/desktop/desktop-model-runtime-bridge.service.spec.ts \
  --include=src/app/desktop/desktop-model-runtime-resource.service.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/tray-panel/tray-panel.spec.ts
npm run build
```

Expected: PASS without starting LEM or loading a model.

- [ ] **Step 6: Run repository-proportional final gates**

From the repository root:

```bash
gofmt -l go/
git diff --check
go vet ./go/...
go tool wails3 task test
cd frontend-ng && npm run build
```

Expected: PASS, or any unrelated pre-existing broad-gate failure recorded with exact command and output while all changed-scope gates remain green.

- [ ] **Step 7: Inspect exact scope and commit**

Run:

```bash
git status --short
git diff --stat origin/main...HEAD
git diff --check
```

Stage only the files from this plan; do not stage `go.work.sum` or `.playwright-mcp/`.

```bash
git add Taskfile.yml build/darwin/Taskfile.yml build/linux/Taskfile.yml build/windows/Taskfile.yml scripts/dev-doctor.mjs scripts/dev-doctor.test.mjs scripts/verify-model-runtime-convergence.test.mjs frontend-ng/package.json docs/development.md AGENTS.md TODO.md
git commit -m "build(modelruntime): package the LEM sidecar"
```

- [ ] **Step 8: Push the completed main branch**

Run:

```bash
git push origin main
```

Expected: the implementation commits and both design/plan documents are present on `origin/main`.
