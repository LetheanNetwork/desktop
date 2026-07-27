<!-- SPDX-License-Identifier: EUPL-1.2 -->

# LEM Model Runtime Bridge Design

**Date:** 2026-07-27

**Status:** Approved for implementation planning

**Scope:** Lethean Desktop-managed, model-less `lem serve` lifecycle and typed
Control/Telemetry integration

## Context

Lethean Desktop currently has three related but distinct responsibilities:

- `lthn serve` hosts the Desktop/CoreGO API;
- `pkg/services` manually supervises optional background processes through the
  named `dappco.re/go/process.Service`; and
- Control and Telemetry combine live Desktop data with labelled model-runtime
  fixtures because there is no typed bridge to the inference host.

The inference host already exists as the `lem` CLI. Help-only discovery against
the local `bin/lem` established the relevant contract without starting a server
or loading a model:

```text
lem serve [--model <path>] [flags]
```

`lem serve` can start model-less when `--model` is omitted. It exposes
OpenAI-, Anthropic-, and Ollama-compatible inference routes, `/v1/models`,
`/v1/health`, admin reload support, optional multi-model residency,
schedulers, KV-cache policies, conversation state, embeddings, reranking, and
ASR. CORS is disabled by default. `lem spec --help` confirms the OpenAPI
document can be exported without loading a model or starting a server.

Desktop must consume that working runtime rather than reproduce it. The
inference sidecar remains optional and manual by default.

## Goals

1. Supervise `lem serve` as a separate managed service named `inference`.
2. Start it without loading a model.
3. Keep `lthn serve` and its existing Desktop API responsibility unchanged.
4. Add a typed Go bridge for health, readiness, loaded models, model
   load/unload, metrics, and bounded history.
5. Drive Control and Telemetry from one shared Angular resource.
6. Preserve deterministic browser demo mode with zero Wails calls and zero
   native event subscriptions.
7. Keep executable paths, model paths, arguments, environment, arbitrary URLs,
   and admin credentials out of the renderer.
8. Route model discovery and selection through `io.Medium`.
9. Work in development and packaged applications without an untrusted PATH
   fallback.
10. Verify the implementation without loading a real model or requiring a live
    LEM endpoint.

## Non-goals

The first implementation does not add:

- a multi-model residency configuration editor;
- scheduler, draft-block, capture, policy, welfare, or state-store settings;
- a remote arbitrary-URL inference host;
- renderer access to LEM's admin token;
- automatic inference startup during Desktop registration or window opening;
- persisted runtime history across a full Desktop restart;
- cloud-model materialisation into a local execution cache;
- raw filesystem fallbacks for model discovery; or
- a replacement for `lthn serve`.

The typed snapshot may represent several loaded models because `/v1/models`
does, but the first Control workflow selects one active model at a time.

## Considered approaches

### 1. Desktop Go adapter over supervised `lem serve` — selected

`pkg/services` owns the child lifecycle. A new `pkg/modelruntime` service owns
the loopback HTTP/admin protocol. Angular calls only typed Wails methods.

This preserves the existing process manager, prevents CORS and admin-token
exposure, supports a separately served GUI later, and isolates LEM protocol
changes behind one Go interface.

### 2. Angular calls LEM directly — rejected

This would require CORS, expose the admin credential inside the WebView, couple
UI lifecycle to a loopback port, and duplicate reconnection/error parsing in
the renderer. It weakens the native transport boundary.

### 3. Fold LEM into `lthn serve` or `pkg/runner` — rejected

This removes a process at the cost of linking Desktop to the heavy inference
runtime, duplicating LEM's server/lifecycle work, and preventing independent
runtime replacement. It also conflates the Desktop API with the model driver.

## Architecture

### Ownership

The responsibilities remain deliberately narrow:

| Unit | Responsibility |
| --- | --- |
| `pkg/services` | Register and supervise the `inference` child; start, stop, restart, output, restart policy, and shutdown |
| `pkg/modelruntime` | LEM protocol, readiness, credentials, safe model operations, snapshots, bounded history, and Core events |
| `pkg/models` | Medium-backed model catalogue and trusted model references |
| Desktop composition | Supply trusted executable and local execution-path capabilities |
| Wails wrapper | Expose renderer-safe model-runtime methods only |
| Angular bridge | Validate every response and reject execution-bearing fields |
| Shared Angular resource | Reconcile snapshots/events once for Control and Telemetry |
| Control/Telemetry presenters | Render state and emit user intents; no transport ownership |

`pkg/modelruntime` depends on interfaces for the services manager, LEM client,
credential provider, model catalogue, execution-path resolver, clock, and
event sink. Tests replace each dependency without spawning a process or
opening a real socket.

### Managed service definition

Desktop adds this trusted built-in definition:

```text
ID:                   inference
Display name:         LEM inference runtime
Kind:                 service
Arguments:            serve --addr 127.0.0.1:36911 --shutdown-timeout 10s
Restart policy:       never
Grace period:         15 seconds
Owner:                lethean
```

The `--model` flag is omitted, which uses LEM's documented model-less default.
No CORS flag is supplied. Desktop does not opt into capture, welfare, a policy
file, a state store, a scheduler, or multi-model configuration.

Registration, catalogue reads, status reads, event subscriptions, Control
opening, and Telemetry opening never call `Start`. Explicit user operations
are the only start authority.

### Executable resolution and packaging

The renderer never chooses an executable. Trusted composition supplies one
explicit candidate:

- development: the repository's `bin/lem`, passed by the development task;
- packaged macOS/Linux: a sibling executable named `lem`;
- packaged Windows: a sibling executable named `lem.exe`.

There is no `exec.LookPath`, shell expansion, current-directory scan, or
renderer override. An absent or non-executable configured candidate fails
closed through the named `go-process.Service` start result as
`binary_missing`; product code does not add a raw filesystem preflight.

Build and packaging tasks stage the matching LEM sidecar and add an executable
convergence test. Optional-source checkout paths may be configurable at build
time, but the resulting runtime location is fixed by the package layout.

### LEM client

`pkg/modelruntime` defines a narrow typed client rather than a generic
renderer-controlled HTTP proxy:

```go
type Client interface {
    Health(core.Context) core.Result
    Models(core.Context) core.Result
    Status(core.Context, AdminCredential) core.Result
    Metrics(core.Context, AdminCredential) core.Result
    Reload(core.Context, AdminCredential, ReloadRequest) core.Result
    Unload(core.Context, AdminCredential) core.Result
}
```

The production client is pinned to `http://127.0.0.1:36911`, uses short
bounded connect/read deadlines, caps response bodies before decoding, rejects
redirects, and accepts only expected JSON content. It does not accept a URL
from configuration or the renderer.

The implementation may export LEM's OpenAPI document during development to
confirm exact request/response shapes. Exporting the spec starts no server and
loads no model. The Desktop service still exposes a narrow hand-reviewed
contract instead of forwarding the generated surface wholesale.

### Admin credential

The admin bearer credential belongs only to Go. LEM itself calls its
fail-closed `EnsureAdminToken` path before binding the listener, generating the
mode-0600 token at `~/Lethean/lem/admin.token` when necessary. Desktop does not
invoke `--print-admin-token`: ordinary `go-process` output is a shared
diagnostic surface and must never capture a secret.

After the managed process becomes healthy, a credential provider reads
`lem/admin.token` through the registered application `io.Medium`, validates the
token shape, and retains a bounded in-memory copy. LEM remains the only writer
and rotator. Desktop never uses raw host-file access or a fallback path.

The token is never:

- returned from Wails;
- placed in the service catalogue;
- included in Core events;
- added to audit metadata;
- included in an error string;
- logged; or
- persisted by Desktop.

An absent or unreadable Medium/token fails closed without an admin request. An
admin HTTP 401 invalidates the in-memory value, re-reads the Medium once, and
permits exactly one request retry. A second rejection fails closed as
`admin_unauthorised`.

## Medium-backed model boundary

### Safe model identity

Angular works with a stable model ID, never a path. The trusted Go-side
catalogue retains:

```go
type ModelReference struct {
    ID           string
    DisplayName  string
    MountID      string
    RelativePath string
    Format       string
}
```

Renderer views omit `MountID` and `RelativePath`. A renderer load request is:

```go
type LoadRequest struct {
    ModelID string `json:"modelId"`
}
```

Model IDs use the repository's existing bounded identifier convention.

### File access

All catalogue reads, metadata inspection, imports, and model selection flow
through registered `io.Medium` instances. There is no `os`,
`path/filepath`, `syscall`, `core.Stat`, `core.ReadDir`, or Core-Fs fallback.
An unavailable Medium makes the catalogue unavailable.

The existing absolute-path model response is not used by the new bridge.
Control migrates to the path-safe catalogue view. Any `pkg/models` behaviour
touched by the implementation must be converted to the registered service and
Medium contract; it must not preserve an absolute-path renderer escape hatch.

### Native execution path

LEM ultimately needs a native path for a locally executable model. Desktop
composition therefore provides a trusted `ExecutionPathResolver` associated
with locally composed model mounts. Resolution:

1. validates the model ID through the catalogue;
2. validates the provider-relative path through the Medium;
3. requires an explicit local-execution capability for that mount;
4. constructs the native child path only inside trusted Go; and
5. passes it directly to the LEM admin client.

Remote, object-store, database, encrypted, or memory Media remain browseable
but return `model_not_loadable` until a separate audited materialisation
pipeline copies content through Medium into a local execution cache.

The resolved native path is transient and never appears in renderer DTOs,
Core events, audit metadata, or public errors.

## Runtime contract

### States

The closed runtime state set is:

```text
unavailable
stopped
starting
model-less
loading
ready
degraded
failed
stopping
```

Transitions are:

```text
unavailable
    └─ dependency becomes available → stopped

stopped
    ├─ explicit Start → starting
    └─ explicit Load → starting

starting
    ├─ health ready, no model → model-less
    ├─ load requested → loading
    └─ deadline/failure → failed

model-less
    ├─ Load → loading
    └─ Stop → stopping

loading
    ├─ selected model ready → ready
    ├─ operation failure → failed
    └─ Stop → stopping

ready
    ├─ Unload → model-less
    ├─ another Load → loading
    ├─ health lost → degraded
    └─ Stop → stopping

degraded
    ├─ health recovers → ready or model-less
    ├─ explicit Restart → starting
    └─ Stop → stopping

failed
    ├─ explicit Start/Restart/Load → starting
    └─ Stop → stopping

stopping
    └─ process exit → stopped
```

Process liveness alone never produces `ready`.

### Renderer-safe snapshot

The renderer receives an immutable view equivalent to:

```go
type Snapshot struct {
    State          State
    Desired        bool
    ActiveModelID  string
    Models         []ModelView
    Metrics        MetricsView
    History        []SampleView
    RefreshedAt    string
    LastHealthyAt  string
    Stale          bool
    LastError      *FailureView
}
```

`ModelView` contains only ID, display name, load state, runtime, context,
capabilities, loaded time, and safe availability labels. `MetricsView` and
`SampleView` use optional fields or explicit availability; unsupported values
are absent, not zero.

Every slice, string, response, history, and error message is bounded before it
crosses Wails.

The Wails service name is `ModelRuntime`. Its complete renderer method set is:

```text
Snapshot()
Start()
Load({modelId})
Unload()
Restart()
Stop()
```

There is no generic request method, URL parameter, command parameter, argument
parameter, path parameter, token method, or raw LEM response method.

### Stable failures

The closed renderer-safe codes are:

```text
runtime_unavailable
binary_missing
runtime_stopped
runtime_start_failed
runtime_not_ready
catalogue_unavailable
model_not_found
model_not_loadable
model_load_failed
model_unload_failed
admin_unauthorised
operation_in_progress
response_invalid
response_too_large
request_timeout
runtime_stop_failed
```

Internal causes may wrap richer errors, but the renderer receives only a
bounded code and calm British-English message.

## Operations and data flow

### Snapshot

1. Read `inference` from `pkg/services`.
2. If it is unavailable or stopped, return immediately without an HTTP call.
3. If it is starting, loading, or stopping, merge the operation state with the
   last good LEM data.
4. If it is running, call health first.
5. On health success, collect models, status, and available metrics through
   the typed client.
6. Validate and bound the response, update last-good state, append a sample,
   and return a fresh snapshot.
7. On a transient failure, retain last-good data, mark it stale, and return a
   degraded snapshot.

### Explicit Start

1. Serialise against other mutations.
2. Call `pkg/services.Start("inference")`.
3. Poll health with bounded exponential backoff until the readiness deadline.
4. Return `model-less` when healthy.

### Explicit Load

The Load button is explicit authority to start the optional runtime:

1. Validate the model ID.
2. Resolve its Medium-backed reference and local execution capability before
   starting anything.
3. If stopped, start `inference`.
4. Wait for process health.
5. Read the LEM-owned admin credential through the registered application
   Medium.
6. send the trusted reload request with the native model path;
7. poll status/models until the requested ID is ready;
8. append the first runtime sample; and
9. publish an invalidation event.

Failure after Desktop started the process does not silently stop it. The
runtime remains model-less or failed, its output stays available through the
Services interface, and the user can retry or stop explicitly.

### Explicit Unload

Unload serialises against other mutations, invokes the typed admin operation,
waits for a healthy model-less state, clears active-model metrics, retains
bounded historical samples, and publishes an invalidation.

### Explicit Stop and Restart

Stop and Restart delegate child lifecycle to `pkg/services`. A stop during a
load cancels readiness polling and then performs the managed graceful
shutdown. Restart returns to model-less state; it does not implicitly reload
the previous model.

Closing a Control or Telemetry window does not stop the runtime. Desktop/Core
shutdown continues to stop all managed children.

## Sampling, events, and history

While `inference` is desired and running, `pkg/modelruntime` samples every five
seconds. It keeps at most 720 samples, representing one hour. Samples live
only in memory for this slice.

Sampling never starts the process and never loads a model. The sampler stops
when the runtime stops or Core shuts down.

State, model, error, and sample changes emit:

```text
lthn:model-runtime:changed
```

The event payload is invalidation metadata only:

```text
reason
state
at
```

It carries no metrics, model path, command, arguments, output, URL, or token.

Angular keeps a slower bounded snapshot poll as recovery for a missed event.
Events are the primary refresh path.

Start, Load, Unload, Restart, and Stop emit requested/succeeded/failed audit
lifecycles. Audit metadata may carry the fixed runtime literal, operation,
stable failure code, and a hash of the model ID. It never carries the model
path, token, endpoint, command, arguments, output, prompt, or response.

## Angular integration

### Bridge

`DesktopModelRuntimeBridgeService` owns exact Wails method names, offline
guards, request validation, strict parsers, and the native event source.

It rejects unknown states, non-finite numbers, oversized collections, invalid
timestamps, and any response containing execution-bearing or secret-like
fields such as:

```text
path
command
arguments
environment
workingDirectory
endpoint
url
token
secret
credential
key
```

Offline mode throws before any Wails call and does not subscribe to events.

### Shared resource

A root-scoped `DesktopModelRuntimeResource` owns one immutable
`DesktopDataResource<RuntimeView>` and one native listener for all consumers.
The first live consumer starts reconciliation; subsequent consumers share it.
The last consumer removes the Angular listener and fallback timer. The Go
sampler may continue while the explicitly started runtime is alive.

The resource:

- preserves last-good data during refreshes and failures;
- exposes pending operation IDs;
- coalesces event bursts;
- ignores late settlements after teardown;
- distinguishes demo, live, stale, mixed, and unavailable data; and
- never substitutes demo metrics into a connected snapshot.

### Control

Control's Models surface replaces benchmark-derived headline fixtures with
runtime data:

- active model and runtime state;
- prompt and decode throughput;
- active/peak memory when available;
- uptime and request activity when available;
- bounded live history;
- path-safe available/loaded model rows; and
- explicit Start, Load, Unload, Restart, and Stop actions appropriate to the
  current state.

The existing Services view remains the lower-level process/output interface.

### Telemetry

Telemetry consumes the same resource for model throughput, runtime memory,
active model, runtime, KV-cache, and uptime fields. Desktop-process heap and
future host-power measurements remain separate sources and are labelled as
such.

### Demo mode

Demo mode uses fresh deterministic in-memory data for each browser session.
It visibly labels LEM data as demonstration data, supports safe simulated
state transitions, and executes no native process, HTTP request, Wails call,
or native event subscription.

## Concurrency and lifecycle

One mutation lock serialises Start, Load, Unload, Restart, and Stop. A
generation token binds readiness and sampling results to the process/model
generation that produced them. Late results from an earlier generation are
discarded.

Read-only Snapshot calls may run concurrently but update history through one
bounded state lock. They never hold a lock across an HTTP request.

Core shutdown:

1. rejects new mutations;
2. cancels health/readiness/sampling contexts;
3. waits for in-flight modelruntime work to leave its critical sections;
4. releases the in-memory credential; and
5. delegates child shutdown to `pkg/services`.

## Testing strategy

### Go tests

Focused Good/Bad/Ugly tests use:

- a fake managed-services boundary;
- a fake typed LEM client;
- a fake credential provider;
- `io.MemoryMedium`;
- a fake execution-path resolver;
- a fake clock; and
- a bounded event recorder.

Required proofs:

1. Registration, Snapshot, event subscription, and sampler construction never
   start `inference`.
2. Explicit Start performs start then bounded health readiness.
3. Explicit Load performs model validation, optional start, health,
   credential, reload, and readiness in that order.
4. Invalid/non-loadable model IDs fail before process start.
5. Unload leaves a healthy model-less process.
6. Restart does not reload the previous model.
7. Stop cancels a concurrent load and uses the managed lifecycle.
8. Health loss produces degraded/stale data without deleting last-good state.
9. One 401 refreshes the credential once; a second fails closed.
10. Malformed, redirected, oversized, late, and timed-out responses are
    rejected.
11. Concurrent mutations return `operation_in_progress`.
12. History never exceeds 720 samples.
13. Events and Wails results contain no secret or execution-bearing fields.
14. Medium absence and binary absence fail closed.
15. Core shutdown stops sampling and releases the credential.

The production HTTP client uses an in-process test server only. Tests never
start the real `lem`, load weights, or depend on port 36911.

### Angular tests

Required proofs:

1. Offline mode performs zero Wails calls and zero event subscriptions.
2. Strict parsing rejects unknown states, invalid numbers, excessive rows, and
   secret/path-bearing payloads.
3. The shared resource creates one listener and one fallback timer for
   multiple consumers.
4. Event bursts coalesce into one refresh.
5. Last-good values survive transient errors and become visibly stale.
6. Pending operations disable conflicting actions.
7. Late responses after teardown are ignored.
8. Control renders state-appropriate actions and no native paths.
9. Telemetry and Control consume the same runtime snapshot.
10. Demo actions mutate only the demo resource.

### Build and packaging tests

Convergence tests prove:

- the canonical product frontend remains `frontend-ng`;
- development passes an explicit repository `bin/lem`;
- packaged applications stage `lem`/`lem.exe` beside the Desktop executable;
- production resolution has no PATH fallback;
- the managed definition binds only loopback port 36911;
- CORS remains disabled; and
- no build/test command loads a model.

## Acceptance criteria

The slice is complete when:

1. A fresh Desktop boot shows inference as stopped without spawning `lem`.
2. Start launches a healthy model-less `lem serve` on loopback.
3. Load accepts a model ID, resolves it through Medium in Go, and reaches ready.
4. Unload returns to model-less state without stopping the service.
5. Stop and Desktop shutdown terminate the managed process.
6. Control and Telemetry display the same truthful runtime state and history.
7. Unsupported values display as unavailable rather than zero or demo data.
8. Offline demo mode remains fully usable with no native traffic.
9. No renderer contract or event exposes paths, execution data, URLs, or
   credentials.
10. Focused Go, race, vet, Angular, build, and convergence checks pass without
    loading a model.

## Follow-on work

After this slice is stable, separate designs may add:

- multi-model residency/profile editing;
- scheduler and MTP controls;
- persisted runtime history;
- provider-native queue and request telemetry where not already exposed;
- Medium-to-local-cache model materialisation;
- capture-dataset management;
- outbound policy and welfare configuration; and
- per-service CPU/memory sampling that can be proven locally and then lifted
  into `go-process`.
