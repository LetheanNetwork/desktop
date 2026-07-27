<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Managed Service Signals Design

## Goal

Give the managed service manager the two operations it is missing when a
service will not stop politely: send a named signal, and kill the process
tree.

Control currently offers Start, Stop and Restart. All three are graceful —
`Stop` asks and waits. A service that has stopped answering has no route back
except quitting Lethean Desktop, which is the one thing a service manager
exists to avoid.

## Current problem

`TODO.md` asks for one item covering five things:

> Extend the process surface with PID, CPU, memory, start time, and
> signal/kill actions.

Three of the five are already there. `services.Snapshot` carries `PID`,
`StartedAt` and `ExitCode`, and has since the manager landed. What is missing
divides on a line that matters, because the two halves live in different
repositories:

- **Signals and kill** need nothing new anywhere. `dappco.re/go/process`
  v0.16.1 already exposes `Service.Signal(id, sig)`, `Service.SignalPID`,
  `Service.Kill(id)` and `Service.KillPID`, and the process-group variants
  underneath them. The manager simply does not call them.
- **CPU and memory** have no source. `process.Info` carries ID, command,
  args, dir, started-at, running, status, exit code, duration and PID — and
  nothing about resource use. There is no `Usage`, `Stat` or `Rusage` surface
  anywhere in the package. Per-service CPU and memory therefore begins as a
  change to `dappco.re/go/process`, a library the whole fleet consumes, and a
  version bump after it.

This design covers the first half only. The second is named here so that the
`TODO.md` item is not read as one job, and so that nobody adds a CPU column
by shelling out to `ps` because the library appeared not to support it.

## Approaches considered

**Expose `Kill` alone.** Simplest, and wrong on Unix: `SIGKILL` cannot be
caught, so a service is denied any chance to flush state or remove a socket.
Every real service manager offers a graduated response, and offering only the
brutal one trains people to reach for it.

**Expose an arbitrary signal number.** Rejected. The renderer would then be
choosing kernel constants, the contract would differ per platform, and the
existing rule that "renderer operations address a known service ID only; they
do not accept arbitrary command, argument, environment, or absolute
working-directory values" exists for exactly this reason. A signal number is
the same category of value.

**Expose a small named set.** Chosen. `terminate`, `interrupt`, `hangup` and
`kill` — four names with stable meanings on both platforms, mapped Go-side.
The wire carries a name; the kernel constant never crosses the boundary.
Windows has no signals, so the mapping there is documented rather than
pretended: `terminate` and `kill` both end the process, `interrupt` and
`hangup` are refused with a typed failure rather than silently doing nothing.

## Signal model

```go
// Signal is a named, platform-mapped signal. The wire never carries a
// kernel constant.
type Signal string

const (
    SignalTerminate Signal = "terminate" // SIGTERM — ask politely
    SignalInterrupt Signal = "interrupt" // SIGINT  — as if from a terminal
    SignalHangup    Signal = "hangup"    // SIGHUP  — reload, by convention
    SignalKill      Signal = "kill"      // SIGKILL — cannot be caught
)
```

`kill` is a distinct operation rather than a signal name at the Wails
boundary, because it means something different to the manager: it ends the
whole process tree and marks the service stopped without waiting. The other
three are delivered and nothing else is concluded — a service that ignores
`hangup` stays running, and the manager must not pretend otherwise.

## Ownership boundary

Unchanged, and this design must not widen it:

- Every delivery goes through the named `*dappco.re/go/process.Service` via
  the existing `ProcessRuntime` adapter. The manager does not touch
  `syscall`, `os.Process` or the package-global process runtime.
- The legacy native controller in `manager.go` — launchd and systemd — is not
  reachable from any of this. Signals apply to managed processes only.
- Nothing new is persisted. A signal is an event, not state; the catalogue
  continues to hold definitions and policy alone.

## Restart policy interaction

This is the part that will be got wrong if it is not written down.

A managed service with a restart policy is watched by an exit reconciler. If
a signal ends the process, the reconciler sees an exit and restarts it — so
`kill` would appear to do nothing, or worse, appear to work and then be
undone a second later.

So `Kill` clears desired-running state before it delivers, exactly as `Stop`
does, and the resulting exit is attributed to the operator rather than
counted as a failure against the backoff. A bare signal does **not** clear
desired state: if `terminate` causes a well-behaved service to exit and the
policy says restart, restarting is correct and expected.

## Failure contract

Typed, and specific enough to act on:

- unknown service ID — the existing not-found failure
- service not running — refused; there is no process to signal, and treating
  it as success would make a broken script look fine
- unsupported signal on this platform — refused, naming the platform
- delivery refused by the operating system — the underlying failure, kept

## Wails surface

Two operations, addressed by known service ID:

```go
func (service *WailsService) Signal(request SignalRequest) core.Result
func (service *WailsService) Kill(id string) core.Result
```

`SignalRequest` carries the service ID and the signal name. Both emit the
existing `services.Event` so `lthn:services:changed` invalidates the
renderer's state the same way Start and Stop already do, and both emit the
audit trio.

## Angular surface

The bridge gains `signal(id, name)` and `kill(id)` with the same defensive
parsing and offline guard as the existing mutations. Control's Services view
gains them behind a per-row overflow control rather than as top-level
buttons: they are the unusual response, and a `kill` sitting beside `stop` at
equal weight invites a click that costs someone their unsaved state.

Offline demo mode makes no call, as with every other mutation.

## Out of scope

- CPU and memory. They begin in `dappco.re/go/process`; see above.
- Signalling native launchd or systemd services.
- Arbitrary signal numbers, now or later.
