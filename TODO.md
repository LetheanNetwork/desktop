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

- [ ] Add a capability-scoped directory listing contract for browsing beneath
      approved locations. The current Office Files service provides saved
      locations and recent direct files, not general traversal.
- [ ] Add safe file open, reveal-in-host-file-manager, and preview contracts.
- [ ] Add create folder, rename, move, copy, trash, restore, and delete
      operations with explicit confirmation and conflict results.
- [ ] Add filesystem watch events so visible folders and recents update without
      polling.
- [ ] Add user-managed saved locations and removable/network volume discovery.
- [ ] Add pagination, search, metadata, MIME/type detection, and thumbnail
      generation for large locations.

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
