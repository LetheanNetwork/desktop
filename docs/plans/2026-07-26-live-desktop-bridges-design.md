<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Live desktop bridges and browser demo mode

## Goal

Begin replacing the polished Control, Telemetry, Files, and Terminal fixtures
with truthful Wails-backed data without making backend availability a
prerequisite for frontend design work.

The existing application layouts and interactions remain intact. Data
provenance becomes explicit so a fixed design value is never presented as a
live reading.

## Runtime modes

The current `ConnectionManagerService` remains the transport authority.

- When the transport is explicitly offline, including
  `?lthn-offline=1`, the desktop enters **demo mode**. Applications use the
  typed demo fixtures, make no Wails calls, and show a compact `Demo data`
  indicator.
- When the transport is not explicitly offline, applications attempt their
  live bridges. They show `Loading live data`, then `Live data` when all visible
  data is live or `Live + demo` when unsupported design fixtures remain.
- If a live call fails, the current demo content remains available and is
  labelled `Live unavailable · demo shown`. A backend outage must not collapse
  the application shell or remove useful design surfaces.

Disconnected and reconnecting transports do not silently become demo mode.
Only the explicit offline transport selects demo mode; ordinary connection
failures remain visible failures.

## Shared data boundary

A root-provided Angular data service owns the Wails method names, strict
response parsing, and runtime-mode guard. Components consume typed frontend
models rather than handling unknown bridge payloads.

The service exposes focused reads for:

- the local process telemetry sample;
- local model catalogue entries;
- recent benchmark runs;
- tracked build/process entries;
- the curated desktop setting catalogue;
- saved file locations, recent files, and disk usage.

Each parser rejects malformed data instead of inventing replacement live
values. The Control aggregate uses settled reads so one unavailable subsystem
does not hide successful data from another.

## Application behaviour

### Control

Control keeps its rail, tabs, cards, charts, tables, and configuration layout.
In connected mode:

- model rows come from the local model catalogue;
- benchmark charts and rows come from benchmark history;
- runtime memory, goroutine count, and uptime come from telemetry;
- process rows come from the build service's shared process registry;
- configuration rows come from the curated appconfig catalogue.

Power history, per-model VRAM/KV cache, request rate, CPU history, and daemon
health remain demo fixtures until matching backend contracts exist. Control is
therefore labelled `Live + demo` when any of those sections are visible.

### Telemetry

The existing visual composition remains. Connected mode shows process heap
allocation, goroutine count, GC pause, CGO call count, and process uptime from
`telemetry.Service.CurrentSample`. A bounded in-memory history drives the
sparklines while the window is open.

Power is shown live only when the backend reports a non-zero measurement.
Runner token throughput, active model, region, VRAM, and KV-cache remain demo
data and are tracked in `TODO.md`.

### Files

Connected mode uses the Office Files service for saved locations, recent files,
and disk usage. The current grid/list presentation and sidebar remain.
Arbitrary directory traversal is not fabricated: locations expose their real
recent files, while nested demo folders are available only in demo mode.

General directory browsing, filesystem mutation, file opening, and filesystem
watch events are tracked as backend work in `TODO.md`.

### Terminal

The base Terminal application reuses the existing tested xterm/Wails PTY
surface in connected mode. It does not grow a second terminal implementation.

In demo mode the route keeps the existing typed terminal fixture so browser
design and interaction work does not attempt to open a local shell. The mode
switch sits in a small route component, leaving the PTY implementation shared.

## Testing

Tests are written before production changes and cover:

- explicit offline mode makes no bridge calls;
- strict normalisation of each live response;
- partial Control success preserves truthful live sections;
- malformed or rejected responses retain labelled demo data;
- Files maps saved locations, recent rows, and disk usage into the existing UI;
- the base Terminal route selects demo versus live presentation correctly;
- registry and lazy-route contracts continue to resolve.

Focused Angular tests run during development, followed by the frontend
confidence gate, production build, and `git diff --check`.

## Deferred backend work

No new Go data contracts are added in this tranche. Missing sources are
recorded in the root `TODO.md` with the exact UI consumers they would unlock.
This keeps frontend design work fast while making the path from demo data to
live data explicit.
