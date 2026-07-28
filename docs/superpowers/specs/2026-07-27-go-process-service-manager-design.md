<!-- SPDX-License-Identifier: EUPL-1.2 -->

# `go-process` Service Manager Design

## Goal

Make `go/pkg/services` the single Lethean Desktop owner for optional
background applications, services, and processes.

The manager uses the already-registered `dappco.re/go/process` service for
process lifecycle, persists its catalogue through the application
`io.Medium`, exposes typed Go and Wails operations, and powers Control's
existing Daemons view. Nothing is started merely because Lethean Desktop
starts. A user or trusted Lethean package must explicitly request a start.

Managed processes remain alive while the desktop has no open windows and its
tray runtime is still active. An explicit Lethean Desktop shutdown gracefully
stops them. Surviving an explicit quit, login, reboot, or manager crash is a
separate native-host policy and is not silently enabled by this manager.

## Current problem

The repository has two lifecycle surfaces which do not form one service
manager:

- `go/pkg/services` has a static `serve` and `tray` registry. Its free
  functions write launchd or systemd definitions and delegate lifecycle to the
  operating-system service manager. Installing a definition also enables
  automatic login startup.
- the named `dappco.re/go/process.Service` registered by
  `go/cmd/lthn/app.go` starts and tracks processes for the current Core
  runtime. `go/pkg/process` adds CLI, API, and audit adapters around that
  process registry, but it has no durable Lethean service catalogue or
  service-level desired state.

The Angular Control application therefore has:

- a live process table with only the generic process registry fields;
- a Daemons table made entirely from labelled fixture data;
- no working start, stop, restart, status, or output actions; and
- no central place for optional Lethean capabilities to declare that they can
  run as a background service.

Using launchd or systemd for every optional component would make installed
features start at login even when the user never uses them. Starting directly
through `go-process` from each package would instead create multiple lifecycle
owners with inconsistent status, shutdown, output, and restart behaviour.

## Compatibility contract

The implementation must preserve these existing product decisions:

- the `lthn` binary remains a CLI router first; adding the manager must not
  make unrelated CLI verbs depend on Angular or an open desktop window;
- the named Core service `process` remains the only runtime which spawns and
  owns managed child processes;
- `go/pkg/process` retains its generic process CLI and Wails-only API surface;
- the existing explicit launchd/systemd `install` and `uninstall` capability
  remains isolated for packaging and native-host work;
- ordinary service-manager `Start` never installs, enables, or invokes an OS
  login service;
- no managed definition starts during Core registration or startup;
- explicit offline transport makes no Wails call or event subscription and
  retains a useful, visibly labelled services demo;
- Control retains its route, window behaviour, System tabs, process view, and
  existing fixture experience;
- renderer contracts never receive provider roots, absolute persistence paths,
  environment secrets, or unbounded process output;
- all new catalogue persistence and recovery operations flow through the
  registered application `io.Medium` and fail closed when it is unavailable;
- British English, EUPL-1.2, and the project's no-paywall policy remain in
  force.

## Approaches considered

### Core service wrapping the registered `go-process` runtime — selected

Create a canonical `services.Service` with durable definitions and
service-level runtime state. It delegates every spawn, observation, output
read, graceful stop, and process-tree kill to the existing named
`go-process.Service`.

This gives internal packages and the desktop one lifecycle owner without
creating another always-running process.

### Add managed operations beside the static OS controller

New free functions could start processes while the old functions continue to
start native services. This is superficially smaller, but `Start`, `Stop`, and
`Status` would have two meanings and two registries. Callers could not know
which owner was authoritative.

### Introduce a separately installed supervisor daemon

A dedicated `lthn service host` could survive Desktop quit and manage
processes for both CLI and GUI clients. It would require authenticated local
IPC, crash adoption, installation, upgrade coordination, and its own lifecycle
before the first useful service can run. That may become an explicit
native-host mode later, but it contradicts the current requirement not to
start background infrastructure by default.

## Ownership boundary

`go/pkg/services` gains a CoreGO service with the repository's canonical
shape:

```go
type Service struct {
    // private dependencies, catalogue, runtime records, and locks
}

type Options struct {
    Process    ProcessRuntime
    Catalogue Catalogue
    Builtins  []Definition
    Limits    Limits
}

func NewService(options Options) *Service
func (s *Service) Register(c *core.Core) core.Result
func Register(c *core.Core) core.Result
```

The free `Register` function:

1. resolves the named `*process.Service` from Core;
2. resolves the application `*io.Service` and its `Medium`;
3. creates a Medium-backed catalogue at
   `desktop/services/catalogue.json`;
4. composes the built-in definitions; and
5. returns the registered `*services.Service`.

`go/cmd/lthn/app.go` registers `services` after both `process` and `io`.
Registration constructs and validates the manager but does not start a
managed process.

Production uses a narrow `ProcessRuntime` adapter over
`*dappco.re/go/process.Service`. Tests use a deterministic fake implementing
the same calls. The adapter exists for fault injection and does not provide a
second process implementation.

The current native launchd/systemd controller remains a separate compatibility
adapter. Its functions are not called by `services.Service.Start`,
`Stop`, `Restart`, `Status`, or `Output`. A later native-host design may rename
those operations more explicitly, but this tranche does not make the manager
depend on native installation.

## Definition model

A definition describes one trusted background capability:

```go
type Kind string

const (
    KindService Kind = "service"
    KindApp     Kind = "app"
    KindProcess Kind = "process"
)

type RestartPolicy string

const (
    RestartNever     RestartPolicy = "never"
    RestartOnFailure RestartPolicy = "on-failure"
    RestartAlways    RestartPolicy = "always"
)

type Definition struct {
    ID               string
    DisplayName      string
    Description      string
    Kind             Kind
    Command          string
    Arguments        []string
    WorkingDirectory WorkingDirectory
    RestartPolicy    RestartPolicy
    GracePeriod      time.Duration
    Owner            string
}
```

The exact field names may follow repository serialisation conventions, but the
wire representation remains explicit and typed.

Definition rules:

- `ID` is a stable, lower-case Lethean identifier and is the only lifecycle
  address accepted from Angular;
- display name, description, kind, owner, restart policy, and grace period are
  safe catalogue metadata;
- command and arguments remain server-owned definition data;
- arbitrary command bytes, arguments, environment values, and native working
  directories are never accepted by `Start`;
- a renderer can start only an ID returned by the manager's catalogue;
- definitions are immutable while their service is starting, running,
  stopping, or awaiting restart;
- duplicate IDs, unknown kinds or policies, empty commands, invalid durations,
  and unresolvable working-directory references fail validation before
  persistence;
- a definition cannot address the running Desktop or tray process in a way
  that recursively launches another desktop host.

`WorkingDirectory` is a trusted reference, not an absolute renderer path. A
renderer-facing value contains only a registered mount or workspace identifier
and a provider-relative path. Trusted Go composition resolves it for a local
process only after the associated Medium validates the path and declares that
native process execution is supported. Remote/object media return a typed
unsupported result. Absolute provider roots never cross Wails.

Environment overrides are deliberately absent from the first renderer
contract. A process inherits the manager's approved environment. Future
overrides must distinguish non-secret values from references resolved through
the existing credentials service; plaintext secrets must not be stored in the
catalogue, returned to Angular, or written to audit metadata.

Trusted Go packages can ensure or remove definitions through the manager. The
first Wails tranche manages lifecycle and restart policy for known
definitions; it does not turn a remotely reachable renderer method into an
arbitrary persistent command-registration primitive. A user-defined command
editor requires a separate local-execution consent and authority design.

## Durable catalogue

The application I/O Medium stores a versioned document:

```text
ServiceCatalogueDocument
  version
  definitions[]
  policyOverrides[]
  updatedAt
```

Compiled built-ins and package-owned definitions are merged by ID with the
Medium-backed document. Ownership determines who may replace or remove a
definition. A user policy override may change restart behaviour or grace
period within server-owned bounds, but it cannot replace a command or claim
another package's ID.

Writes use a staging document, validation read-back, rename, and recoverable
backup through `io.Medium`. Startup recovery accepts only a complete supported
version. A malformed document, unsupported version, failed recovery, or
unavailable Medium places the manager in an unavailable state: it does not
start anything, invent an empty catalogue, overwrite the evidence, or fall
back to raw host file access.

Runtime desired state is not persisted. In particular, `running` is never
interpreted as “start this again next time”. Reopening Lethean Desktop loads
definitions and policies in the stopped state.

Process IDs, PIDs, output, and transient errors remain runtime observations.
They are not written to the catalogue.

## Runtime model

The manager presents immutable snapshots:

```go
type State string

const (
    StateStopped  State = "stopped"
    StateStarting State = "starting"
    StateRunning  State = "running"
    StateStopping State = "stopping"
    StateExited   State = "exited"
    StateFailed   State = "failed"
)

type Snapshot struct {
    Definition   DefinitionView
    State        State
    Desired      bool
    ProcessID    string
    PID          int
    StartedAt    time.Time
    StoppedAt    time.Time
    ExitCode     int
    RestartCount int
    LastError    ErrorView
}
```

`DefinitionView` excludes execution-only fields which the UI does not need.
`ErrorView` carries a stable code and reader-facing message, never a raw error
object or process output.

The public service surface is:

- `Catalogue() core.Result`;
- `Get(id string) core.Result`;
- `Start(id string) core.Result`;
- `Stop(id string) core.Result`;
- `Restart(id string) core.Result`;
- `Output(request OutputRequest) core.Result`;
- trusted-Go definition registration and removal operations; and
- `OnStartup` / `OnShutdown` Core lifecycle hooks.

All returned slices and nested data are copied so callers cannot mutate the
manager's state.

## Start lifecycle

`Start` is explicit and idempotent:

1. validate the service ID and manager availability;
2. reject an operation already in progress;
3. return the current snapshot when it is already running;
4. resolve the trusted working directory;
5. mark desired state true and state starting;
6. call `go-process.Service.StartWithOptions` with output capture, a bounded
   grace period, detached process grouping, and whole-group termination;
7. require the returned value to be a managed `go-process.Process`;
8. record its process ID, PID, start time, and running state; and
9. start one generation-bound exit observer.

The manager does not invoke `launchctl`, `systemctl`, a shell wrapper, or a raw
`os/exec` path. `go-process` receives command and argument arrays directly.

Only a successful explicit start sets desired state true. Loading a catalogue,
opening Control, refreshing status, or subscribing to events never does.

## Exit and restart lifecycle

The exit observer waits on the `go-process` process and then captures its final
snapshot. Each run receives a monotonically increasing generation. An exit
from an older generation cannot overwrite or restart a newer run.

On exit:

- an explicitly stopped process becomes stopped and remains undesired;
- a zero exit becomes exited unless `RestartAlways` is active;
- a non-zero or failed exit becomes failed;
- `RestartOnFailure` restarts only a non-zero or failed run;
- `RestartAlways` restarts any unrequested exit;
- `RestartNever` never restarts;
- every automatic restart is valid only while desired state remains true and
  the Core is not shutting down.

Automatic restart uses bounded exponential backoff and a restart budget.
Exhausting the budget clears desired state and leaves a stable failed snapshot
instead of creating a hot respawn loop. Merely loading a definition never
creates a restart budget or timer.

## Stop, restart, and shutdown lifecycle

`Stop` clears desired state and invalidates pending restart work before
signalling the process. It obtains the managed process by the stored
`go-process` ID and calls its graceful `Shutdown`, which sends termination and
escalates to whole-process-group kill after the definition's bounded grace
period.

Stopping an already stopped, exited, or failed service is idempotent. A
process lookup mismatch is reported and reconciled to a non-running snapshot;
the manager never signals an unrelated PID merely because a stale integer
matches.

`Restart` is one serialised stop-then-start operation. No overlapping start can
slip between its phases.

During Core shutdown the manager:

1. prevents new starts and automatic restarts;
2. clears desired state for every definition;
3. gracefully shuts down running managed processes in parallel;
4. respects the caller's shutdown context and bounded grace periods;
5. records and joins failures without abandoning the remaining services; and
6. completes before the underlying `go-process.Service` performs its final
   process-tree cleanup.

A normal explicit Desktop quit therefore stops every managed service. Closing
the last window does not trigger this sequence while the Wails tray runtime
remains active.

An abnormal process crash cannot guarantee graceful child cleanup. This first
runtime-scoped manager does not claim to adopt a process after a new Core
starts and does not kill a bare PID without a verifiable `go-process` identity.
Crash-safe adoption belongs with the deferred native-host design.

## Concurrency rules

The manager serialises lifecycle mutations per definition while allowing
independent services to run concurrently.

- catalogue persistence has a separate document lock;
- snapshots never hold a manager lock while waiting for a process or Medium;
- start, stop, restart, definition replacement, automatic restart, and
  shutdown all re-check the run generation after external work;
- a stop racing with process exit resolves to undesired and non-running;
- a shutdown racing with a restart cancels the restart;
- output reads use the currently recorded process ID and cannot cross into a
  newer generation;
- limits bound total definitions, concurrent running services, restart
  attempts, grace periods, and returned output bytes.

## Output contract

`go-process` remains the output owner and captures into its bounded ring
buffer. `services.Output` returns a tail bounded again for Wails, together with
the service ID, process generation, truncation flag, and observation time.

Output is read only after an explicit request. It is not copied into the
catalogue, lifecycle event payloads, errors, telemetry, or audit metadata.
The UI treats it as potentially sensitive and does not persist it.

## Events and audit

Every accepted definition change or runtime transition emits a typed
`services.Event` on the Core ACTION bus. Events contain only:

- service ID;
- operation;
- old and new state;
- desired flag;
- process ID where useful;
- stable error code; and
- timestamp.

`go/pkg/desktop/services_events.go` forwards these events as
`lthn:services:changed`, following the existing Files event adapter. The
renderer treats the event as an invalidation and re-reads the canonical
snapshot rather than trusting event data as complete state.

Start, stop, restart, definition change, automatic restart, and failure also
emit typed audit request/success/failure rows. Audit metadata may contain the
service ID, policy, process ID, bounded error code, and outcome. It must not
contain raw command bytes, arguments, working directories, environment,
output, credentials, or raw error prose.

## Wails contract

`WailsService` becomes a thin wrapper around the Core-registered
`*services.Service`:

```go
func NewWailsService(service *Service) *WailsService
```

It retains the existing `Lifecycle` service identity where compatibility
requires it and exposes:

- `Catalogue`;
- `Get`;
- `Start`;
- `Stop`;
- `Restart`;
- `Output`; and
- bounded restart-policy updates for known definitions.

The wrapper validates DTOs, delegates once, and preserves stable service error
codes. It does not spawn processes itself and does not expose the underlying
`go-process.Process`.

The existing native installer methods remain explicit compatibility
operations and are not used by Angular's ordinary services view. They are not
aliases for managed start or stop.

The current `lthn service` CLI commands retain their native-service meaning in
this tranche. A short-lived CLI Core cannot safely start a runtime-scoped
managed process because its mandatory shutdown would immediately stop that
process. A future managed CLI must talk to an authenticated running Desktop
host or an explicitly installed native service host.

## Angular bridge and view model

Add a focused `DesktopServicesBridgeService` rather than extending the generic
process bridge. It owns Wails method names, request DTOs, defensive parsing,
and the `lthn:services:changed` subscription.

The bridge:

- rejects live calls while explicit offline transport is active;
- accepts only known state, kind, policy, and error-code literals;
- rejects absolute/provider paths, unexpected secret-bearing fields, invalid
  PIDs, unbounded output, and malformed nested records;
- provides `catalogue`, `start`, `stop`, `restart`, `output`, and
  `onChanged`;
- treats events as advisory invalidations; and
- never retries a mutating operation automatically.

Application-local service-manager state models:

- demo, loading, live, stale, and unavailable data truthfully;
- keeps the last successful catalogue after a transient refresh failure;
- records one pending operation per service;
- prevents duplicate button actions while an operation is active;
- refreshes after a successful action or change event; and
- maps stable backend codes to calm British-English messages.

## Control integration

Control's existing System tab value `daemons` remains stable for window and
route compatibility, while its visible label becomes Services.

The Services view retains the useful offline catalogue and adds:

- service name, kind, owner, state, PID, and restart policy;
- Start for stopped, exited, or failed services;
- Stop for running or starting services;
- Restart for running services;
- a bounded output/details panel opened explicitly;
- pending-state feedback and disabled duplicate actions;
- empty, stale, and unavailable presentation; and
- a clear statement that services start manually and stop when Lethean
  Desktop quits.

In explicit demo mode the controls operate only on isolated Angular demo
state. They make no Wails call and do not pretend to launch a host process.
Every demo row and operation remains visibly labelled.

Connected mode never substitutes daemon fixtures for a failed live read. It
retains the last successful snapshot as stale or shows unavailable before the
first successful result.

## Data flow

```text
trusted package definitions ------+
                                  |
application io.Medium catalogue --+--> services.Service
                                             |
explicit Start / Stop / Restart ------------+
                                             |
                                             v
                              named go-process.Service
                                             |
                              Process / Info / Output / Done
                                             |
                                             v
                                  immutable service snapshot
                                             |
                       Core ACTION ----------+---------- audit
                           |
                           v
                 Wails services event
                           |
                           v
          DesktopServicesBridgeService
                           |
                           v
               Control > System > Services
```

No arrow from Core startup reaches `go-process.StartWithOptions`.

## Error model

The service uses stable error codes including:

- `services_unavailable`;
- `catalogue_invalid`;
- `definition_not_found`;
- `definition_invalid`;
- `definition_conflict`;
- `operation_in_progress`;
- `working_directory_unsupported`;
- `running_limit_reached`;
- `process_start_failed`;
- `process_lookup_failed`;
- `process_stop_failed`;
- `restart_budget_exhausted`; and
- `shutdown_incomplete`.

Errors retain their wrapped cause inside Go, but Wails and audit receive only
the stable code and bounded reader-facing message. An unavailable dependency
never falls back to native execution, raw filesystem state, a global process
manager, or an empty persisted catalogue.

## Testing

Implementation uses red-green Good/Bad/Ugly tests.

### Definition and catalogue tests

- valid service, app, and process definitions;
- invalid IDs, kinds, policies, durations, command, and working-directory
  references;
- duplicate and ownership-conflicting definitions;
- immutable returned copies;
- versioned Medium round trip;
- atomic replacement and backup recovery;
- malformed and unsupported documents fail closed;
- unavailable Medium does not fall back or overwrite evidence;
- no runtime/PID/output state is persisted; and
- focused source contract rejects new raw filesystem access in the manager
  data plane.

All tests use `io.NewMemoryMedium()` or a failing Medium fixture and never
write to the real `~/Lethean/` tree.

### Runtime lifecycle tests

- startup performs zero process starts;
- explicit start supplies the expected `go-process.RunOptions`;
- start is idempotent while running;
- failed spawn reaches failed state;
- graceful stop and process-group escalation delegate to `go-process`;
- stop is idempotent when not running;
- restart serialises stop then start;
- exit snapshots preserve exit code and timestamps;
- restart-never, on-failure, and always policies;
- bounded backoff and exhausted restart budget;
- stop cancels a pending automatic restart;
- stale exit generations cannot overwrite a newer run;
- per-service operations serialise while separate services remain independent;
- running limit enforcement;
- output tail bounds and generation isolation;
- shutdown prevents new starts and stops every running service; and
- shutdown aggregates errors without skipping another service.

### Event, audit, and Wails tests

- each transition emits one bounded typed event;
- the desktop forwards it as `lthn:services:changed`;
- events contain no command, arguments, working directory, environment,
  output, provider root, or raw error prose;
- audit request/success/failure lifecycles use stable codes;
- Wails methods delegate to the registered manager;
- malformed inputs and unknown IDs fail before process access; and
- legacy native installation is never invoked by managed lifecycle methods.

### Angular tests

- bridge method names, request shapes, and defensive parsers;
- offline bridge rejects calls and installs no event subscription;
- demo controls mutate only isolated demo state;
- connected initial load, success, empty, failure, stale retention, and
  recovery;
- Start, Stop, and Restart button availability;
- duplicate action suppression and pending feedback;
- successful mutation refresh;
- advisory event invalidation and teardown;
- bounded output display without persistence;
- visible manual-start and Desktop-shutdown policy; and
- the existing Control process view, route, System tab value, and window
  navigation remain unchanged.

## Verification gates

Focused iteration:

```bash
go test ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn -count=1
go vet ./go/pkg/services ./go/pkg/desktop ./go/cmd/lthn

cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-services-bridge.service.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/control/control-secondary-views.spec.ts
```

Before completion:

```bash
gofmt -l go/
git diff --check
go tool wails3 task test:go
go tool wails3 task verify:frontend
cd frontend && npm run build
```

The external CoreGO compliance audit remains a before/after no-regression
diagnostic for changed Go scope. Its existing repository-wide backlog is not
an all-zero acceptance gate.

## Implementation tranches

1. Add the typed manager, Medium catalogue, `go-process` adapter, lifecycle
   state machine, events, audit coverage, and Core registration.
2. Convert the Wails lifecycle binding to the registered manager and add the
   desktop event forwarder.
3. Add the typed Angular bridge and deterministic services demo state.
4. Turn Control's existing Daemons view into the working Services manager
   without changing its route-state value.
5. Run focused and repository confidence gates, then update `TODO.md` and
   `AGENTS.md` only with verified behaviour.

Each tranche is reviewable and leaves no second runtime process owner.

## Deferred work

This design does not:

- install or start a manager at login;
- preserve desired-running state across Desktop launches;
- adopt children after a manager crash;
- convert every optional Core service into a child process;
- expose arbitrary command creation to a renderer;
- persist plaintext environment secrets;
- add HTTP readiness probes, CPU, memory, or accelerator telemetry;
- replace the generic Process table;
- change the current native `lthn service install` packaging workflow; or
- make a short-lived CLI invocation own a long-lived managed process.

Those capabilities can build on the central catalogue and lifecycle contract
without weakening the manual-start default.
