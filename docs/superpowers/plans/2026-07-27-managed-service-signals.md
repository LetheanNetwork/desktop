<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Managed Service Signals Implementation Plan

> **For agentic workers:** implement this plan task-by-task. Steps use checkbox
> (`- [ ]`) syntax. **Tick each box as you complete it** — the previous plan in
> this directory was fully implemented and left entirely unticked, which made a
> finished slice read as untouched work.

**Design:** `docs/superpowers/specs/2026-07-27-managed-service-signals-design.md` — read it first.

**Goal:** Add named signal delivery and process-tree kill to the managed service manager, through to Control.

**Tech Stack:** Go 1.26.2, CoreGO `dappco.re/go` v0.12.0, `dappco.re/go/process` v0.16.1, Wails 3 alpha2.117, Angular 22, Vitest/jsdom.

## Global Constraints

- Work in this repository only. Preserve the existing user-owned `go.work.sum`
  modification and the `.playwright-mcp/` directory; do not commit either.
- Do not use subagents; execute inline.
- Every delivery goes through the named `*dappco.re/go/process.Service` via the
  existing `ProcessRuntime` adapter. Do not import `syscall` outside the signal
  mapping file, do not touch `os.Process`, and do not use the package-global
  process runtime.
- The legacy native controller (`manager.go`, launchd/systemd) is out of reach
  for all of this. Managed processes only.
- Persist nothing new. A signal is an event, not state.
- Renderer operations address a known service ID and a named signal only —
  never a signal number.
- British English in code, copy, docs and tests.
- Real red-green Good/Bad/Ugly tests, `*core.T`, `io.NewMemoryMedium()`, and
  deterministic fakes. Never write to the real `~/Lethean/` tree, and never
  spawn a process that outlives a test.
- `frontend/` is the only product frontend. Offline demo mode makes no Wails
  call.

## File Map

- Modify `go/pkg/services/types.go` — `Signal` type, the four constants, `SignalRequest`, and the two new typed failures.
- Modify `go/pkg/services/types_test.go` — signal validation and failure-shape tests.
- Create `go/pkg/services/signals.go` — name-to-platform mapping, behind build tags where it must differ.
- Create `go/pkg/services/signals_test.go` — mapping and unsupported-platform tests.
- Modify `go/pkg/services/process_runtime.go` — add `Signal` and `Kill` to the `ProcessRuntime` interface and the named implementation.
- Modify `go/pkg/services/lifecycle.go` — `Signal` and `Kill` manager operations, desired-state handling, event and audit emission.
- Modify `go/pkg/services/lifecycle_test.go` (or `restart_test.go` if that is where lifecycle contracts live) — the restart-policy interaction contracts.
- Modify `go/pkg/services/wails.go` — `Signal(SignalRequest)` and `Kill(id)`.
- Modify `go/pkg/services/wails_test.go` — delegation tests.
- Modify `go/pkg/audit/types.go` — audit names for signal and kill.
- Modify `frontend/src/app/desktop/desktop-services-bridge.service.ts` — `signal(id, name)` and `kill(id)`.
- Modify `frontend/src/app/desktop/desktop-services-bridge.service.spec.ts` — wire, parse, offline-guard tests.
- Modify `frontend/src/app/desktop/apps/control/control-services.models.ts` — intents.
- Modify `frontend/src/app/desktop/apps/control/control-services.view.ts` — per-row overflow control.
- Modify `frontend/src/app/desktop/apps/control/control-services.view.spec.ts` — intent tests.
- Modify `frontend/src/app/desktop/apps/control.app.ts` — mutation orchestration.
- Modify `TODO.md` — split the item; tick only the signal/kill half.
- Modify `AGENTS.md` — record the named-signal contract.

---

### Task 1: Name the signals and refuse the rest

- [x] **Step 1: Write failing Good/Bad/Ugly tests** in `types_test.go` and
      `signals_test.go`. Good: each of the four names maps to the expected
      platform value. Bad: an unknown name is refused with the typed failure,
      and a numeric string like `"9"` is refused — it must never be a way in.
      Ugly: on Windows, `interrupt` and `hangup` are refused naming the
      platform, while `terminate` and `kill` are accepted.
- [x] **Step 2: Run the focused tests and confirm the red state.**
- [x] **Step 3: Implement** `Signal`, the four constants, `SignalRequest`, the
      two failures, and `signals.go` with build tags where the mapping differs.
- [x] **Step 4: Run the focused tests and confirm green.**

### Task 2: Deliver through the named process service

- [x] **Step 1: Write failing tests** proving `ProcessRuntime.Signal` and
      `.Kill` delegate to the named `*process.Service` and nowhere else, with a
      deterministic fake. Bad: a nil runtime fails typed rather than panicking.
- [x] **Step 2: Run the focused tests and confirm the red state.**
- [x] **Step 3: Implement** the interface additions and the named
      implementation in `process_runtime.go`.
- [x] **Step 4: Run the focused tests and confirm green.**

### Task 3: Manager operations, and the restart-policy interaction

This is the task most likely to be got wrong — read the design's restart
section before starting.

- [x] **Step 1: Write failing contract tests.** Good: `Kill` on a running
      service clears desired state first, so the exit reconciler does not
      restart it, and the exit is attributed to the operator rather than
      counted against backoff. Good: a bare `Signal` does **not** clear desired
      state, so a policy-managed service that exits on `terminate` is restarted
      as the policy says. Bad: signalling a stopped service is refused, not
      silently successful. Bad: an unknown ID is refused. Ugly: `kill` on a
      service that is already exiting is idempotent and does not double-count a
      restart.
- [x] **Step 2: Run the focused tests and confirm the red state.**
- [x] **Step 3: Implement** `Signal` and `Kill` in `lifecycle.go`, emitting the
      existing `services.Event` and the audit trio for both.
- [x] **Step 4: Run the focused tests and confirm green.**

### Task 4: Expose both through Wails

- [x] **Step 1: Write failing delegation tests** in `wails_test.go`. Good: both
      operations reach the manager with the parsed request. Bad: a malformed
      `SignalRequest` is refused at the boundary rather than reaching the
      manager. Ugly: an unbound manager fails typed rather than panicking.
- [x] **Step 2: Run the focused tests and confirm the red state.**
- [x] **Step 3: Implement** `Signal(SignalRequest)` and `Kill(id)` in
      `wails.go`, plus the audit names in `pkg/audit/types.go`.
- [x] **Step 4: Run the focused tests and confirm green.**

### Task 5: Angular bridge

- [x] **Step 1: Write failing spec tests** for `signal(id, name)` and
      `kill(id)`: correct Wails call shape, strict response parsing, a refused
      unknown signal name, and **no call at all in offline demo mode**.
- [x] **Step 2: Run the focused specs and confirm the red state.**
- [x] **Step 3: Implement** both on the bridge with the existing defensive
      parsing.
- [x] **Step 4: Run the focused specs and confirm green.**

### Task 6: Control surface

- [x] **Step 1: Write failing presenter specs.** The two actions live behind a
      per-row overflow control, not as top-level buttons beside Stop — see the
      design. `kill` carries a confirmation. Demo mode renders them disabled
      and visibly labelled.
- [x] **Step 2: Run the focused specs and confirm the red state.**
- [x] **Step 3: Implement** the intents, the presenter control, and the
      orchestration in `control.app.ts`.
- [x] **Step 4: Run the focused specs and confirm green.**

### Task 7: Verify and record

- [x] **Step 1: Run the full Go suite** — `GOWORK=$(pwd)/../go.work go test ./...`
      from `go/`. Every package must pass; report the count.
- [x] **Step 2: Run the full frontend suite** — `npm run test:ci` in
      `frontend/`. Report files and tests passed.
- [x] **Step 3: Split the `TODO.md` item.** PID and start time are already
      delivered; tick the signal/kill half and leave CPU and memory as their
      own open item, noting that they begin in `dappco.re/go/process`.
- [x] **Step 4: Record the contract in `AGENTS.md`** — named signals only, no
      signal numbers across the boundary, and the restart-policy rule.
- [x] **Step 5: Tick every box in this plan** that you have completed.

---

## Completion note

Implemented by Cladius, 2026-07-27, in this repository on
`codex/managed-service-signals`.

Two departures from the plan, both deliberate:

- **`signals.go` maps on `runtime.GOOS`, not build tags.** The Windows rule —
  that `interrupt` and `hangup` have no meaning there — is worth a test, and
  under build tags that test could only ever run on Windows, which is to say
  never. It runs everywhere now.
- **`ProcessRuntime.Signal` takes the name, not `syscall.Signal`.** The
  package has a boundary test forbidding `syscall`, `os` and `path/filepath`
  so nothing bypasses `io.Medium`. Rather than soften it, the name is carried
  to the last possible moment and `signals.go` is named as the single
  exception — a second file reaching for `syscall` still fails that test.

Gates: Go 88 packages ok / 0 fail. Frontend 72 files, 417 tests passed
(412 before).
