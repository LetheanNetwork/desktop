<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Lethean Desktop TODO

## Live data sources behind the desktop prototypes

Offline transport is an intentional browser-demo mode. Use:

```text
http://127.0.0.1:9245/?lthn-offline=1#/
```

Demo mode must remain useful for UI design, screenshots, interaction tests,
and product demonstrations. It must not attempt Wails calls, and demo values
must stay visibly labelled. Connected mode should progressively replace those
values with truthful live data.

### Control

- [x] Add a path-free model-runtime summary with opaque model IDs, lifecycle
      state, runtime source, and explicit Start/Load/Unload/Restart/Stop
      actions through the central managed-services system.
- [ ] Add a bounded upstream LEM telemetry endpoint for prompt/decode rates,
      request rate, active/peak memory, and KV-cache use. Until LEM exposes
      those measurements, connected Control and Telemetry intentionally render
      `—` and empty chart series rather than copying demo or benchmark values.
- [ ] Extend the process surface with PID, CPU, memory, start time, and
      signal/kill actions. `build.Service.ProcessList` currently exposes only
      tracked process ID, command, state, and exit code.
- [x] Add a typed daemon/service registry with lifecycle state, PID, project
      ownership, bounded restart policy, and start/stop/restart actions.
- [ ] Add provider health/readiness probes and per-service CPU/memory
      telemetry; process liveness is not readiness.
- [ ] Wire the Control configuration inputs and Commit action to
      `appconfig.Service.Set`, including validation, optimistic state, rollback,
      and restart-required feedback.
- [ ] Add a host power source and persisted hourly/daily roll-ups for average,
      peak, idle, and energy-use cards.
- [ ] Add CPU and memory history for the System overview chart.

### Telemetry

- [x] Use the shared path-free runtime snapshot for current model, runtime,
      supported memory/throughput values, and model uptime.
- [ ] Extend the upstream LEM telemetry endpoint with requests per minute,
      queue depth, KV-cache use, and bounded history. The desktop contract is
      already optional-field-safe and must keep unsupported values unavailable.
- [ ] Implement the macOS power helper promised by
      `telemetry.Reading.watts_active` and `watts_idle`; provide equivalent
      platform sources for Windows and Linux.
- [ ] Add GPU/accelerator memory and utilisation through a platform-neutral
      reading contract.
- [ ] Stream or retain bounded telemetry history in Go so reopening the window
      does not start every chart from one sample.
- [ ] Add sample timestamps, source names, and stale-data detection.

### Files

- [x] Browse registered locations through capability-scoped `io.Medium`
      mounts, using only mount IDs and provider-relative paths.
- [x] Provide bounded text/binary preview through `Files.Preview`.
- [ ] Add explicit host open and reveal-in-host-file-manager operations behind
      mount capabilities; neither accepts an absolute renderer path.
- [x] Create folders, rename, copy, move, trash, restore, and permanently
      delete with confirmation, conflict, and partial-result contracts.
- [ ] Add provider-native watch sources. Current `lthn:files:changed` events
      invalidate mutations without polling, but do not observe external
      provider changes.
- [ ] Add Medium-backed user-managed locations plus removable and network
      volume discovery.
- [x] Provide bounded pagination and base entry metadata.
- [ ] Add search, thumbnail generation, richer MIME detection, and indexing
      suitable for very large catalogues.
- [ ] Move configured model-root selection behind its own Medium-backed
      settings source before registering it as a user-managed mount.

### Terminal

- [x] Expose connection/reconnection state and recover or clearly end PTY tabs
      when the Wails transport reconnects.
- [ ] Add a safe, scripted interactive terminal simulator if browser demo mode
      needs more than the current read-only terminal fixture. Never execute local
      commands from an offline browser preview.
- [x] Add persisted terminal workspace/tab metadata without persisting terminal
      contents or secrets by default.

### Shared bridge behaviour

- [ ] Move live app reconciliation to push events where services already emit
      change notifications; keep bounded polling only as a fallback.
- [ ] Standardise live/demo/loading/mixed/unavailable status, last-refreshed
      time, source, retry, and stale-state presentation across all desktop apps.
- [ ] Add end-to-end Wails tests that exercise these four applications against
      a real local host while keeping browser-only component tests deterministic.
