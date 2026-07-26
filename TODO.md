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

- [ ] Add a model-runtime summary contract with loaded/running state,
      generated and prompt token rates, request rate, VRAM, KV-cache use, runtime
      source, and load/unload actions. The current model catalogue only reports
      local file name, path, size, and directory state.
- [ ] Add bounded model and request time-series data so the throughput chart is
      live activity rather than recent benchmark history.
- [ ] Extend the process surface with PID, CPU, memory, start time, and
      signal/kill actions. `build.Service.ProcessList` currently exposes only
      tracked process ID, command, state, and exit code.
- [ ] Add a typed daemon/service registry with health, PID, project ownership,
      restart policy, and start/stop/restart actions.
- [ ] Wire the Control configuration inputs and Commit action to
      `appconfig.Service.Set`, including validation, optimistic state, rollback,
      and restart-required feedback.
- [ ] Add a host power source and persisted hourly/daily roll-ups for average,
      peak, idle, and energy-use cards.
- [ ] Add CPU and memory history for the System overview chart.

### Telemetry

- [ ] Add runner telemetry for current model, generated/prompt tokens per
      second, requests per minute, queue depth, region/runtime, KV-cache use, and
      model uptime.
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

- [ ] Expose connection/reconnection state and recover or clearly end PTY tabs
      when the Wails transport reconnects.
- [ ] Add a safe, scripted interactive terminal simulator if browser demo mode
      needs more than the current read-only terminal fixture. Never execute local
      commands from an offline browser preview.
- [ ] Add persisted terminal workspace/tab metadata without persisting terminal
      contents or secrets by default.

### Shared bridge behaviour

- [ ] Move live app reconciliation to push events where services already emit
      change notifications; keep bounded polling only as a fallback.
- [ ] Standardise live/demo/loading/mixed/unavailable status, last-refreshed
      time, source, retry, and stale-state presentation across all desktop apps.
- [ ] Add end-to-end Wails tests that exercise these four applications against
      a real local host while keeping browser-only component tests deterministic.
