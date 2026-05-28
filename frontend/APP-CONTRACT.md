# lthn-desktop — App Contract (acceptance surface)

The walkable surface of the app, what each part **is**, and what a test should
**assert**. Built by walking the live app via the `lthn-bridge` (eval/click/nav)
+ `lethean-gui shot` (screenshot the native window) — real binding data, not the
mocked e2e harness.

This is the seed for the test suite: most items map to **vitest component tests**
(jsdom, no GUI running) — mount the element, assert it renders + degrades safely.
A few (live actions) need a running runner and are bridge-driven.

## Legend
- ✅ wired + verified (renders, 0 uncaught errors, real bindings)
- 🧩 mockup — renders from hardcoded `FIXTURE_*` data, not wired to a backend
- 🔧 fixed this session
- ⚠️ action not yet exercised (test target — needs real/mocked backend state)
- ❓ not yet walked

## 1. Shell + navigation — ✅
- `lthn-app-shell` mounts at `?surface=app`. 8 views (chat/admin/planning/coding/
  marketing/operations/sales/office), switch via rail, ⌘1–8, ⌘[/], ⌘P palette.
- 15 admin panes selectable via `_select(id)`; per-view + per-pane last-position
  restore (localStorage).
- **Assert:** each view's nav renders; `_select(pane)` mounts the pane element;
  **0 uncaught errors on mount** (this is the headline regression gate — every
  bug found this session was a mount/render crash).

## 2. Admin view — the real, wired app
14/15 panes render clean with live data; 0 errors at top level + across all safe
data sub-tabs (logs/network/fleet/benchmark).

| Pane | Element | State | Notes |
|------|---------|-------|-------|
| Chat | `lthn-chat-window` | ✅ | real conversation + composer |
| Models | `lthn-model-browser-window` | ✅ | local rail + registry; ⚠️ Download |
| Benchmark | `lthn-benchmark-window` | ✅ | runs table + chart; PP/TG/Both tabs ✅; ⚠️ Run, Export |
| Activity (logs) | `lthn-logs-window` | ✅ | Live log / Generation history / Power history tabs ✅; ⚠️ Clear, Pause |
| Audit | `lthn-audit-viewer` | 🔧 | only HTTP/`apiFetch` pane; 401 used to cascade the whole shell to the gate — now the gate is escapable (Back) |
| Telemetry | `lthn-telemetry-window` | ✅ | live readout (t/s, uptime) |
| Processes | `process-panel` | 🔧 | Daemons ✅, **Processes tab froze on "Loading…"** (fixed), Pipelines ✅ |
| Integrations | `lthn-integrations-window` | ✅ | client config cards |
| Tools · MCP | `lthn-tools-window` | ✅ | tool registry + try-it; ⚠️ Add server, Reload |
| Providers | `lthn-providers-window` | ✅ | servers + services grid |
| Network (preview) | `lthn-network-window` | ✅ | This session / Available peers / Ledger tabs ✅; ⚠️ Leave session |
| Fine-tune (preview) | `lthn-distillation-window` | ✅ | LoRA recipe + loss; ⚠️ Run, Browse, Merge, Push to HuggingFace |
| Train (preview) | `lthn-training-window` | ✅ | CL-BPL surface; ⚠️ Start |
| Fleet (preview) | `lthn-fleet-window` | ✅ | Machines / Agents tabs ✅; ⚠️ Configure an agent |
| Settings | `lthn-settings-window` | ✅ | renders; ⚠️ Browse…; ❓ section-nav structure (General/Models/Privacy/…) not enumerated |

## 3. Role views — 🧩 design mockups (NOT wired)
Planning, Coding, Marketing, Operations, Sales, Office render from hardcoded
`FIXTURE_*` consts (e.g. `marketing/campaigns.ts` → `FIXTURE_CAMPAIGNS`, literal
"42 K"/"3.2%"; comment: "arithmetic is the backend's job"). They look complete but
carry no live data. Coding partially reuses the wired Chat + Repos.

- **Contract decision needed:** mark these "preview", or wire them to bindings
  before they count toward the app contract.
- **Assert (until wired):** render-without-crash only; do NOT assert real numbers.

## 4. Untested actions — ⚠️ test targets
Buttons not exercised this walk (mutate state / need backend / explicit-permission):
Models·Download · Benchmark·Run/Export · Logs·Clear/Pause · Tools·Add server/Reload ·
Network·Leave session · Fine-tune·Run/Browse/Merge/Push · Train·Start · Fleet·Configure ·
Settings·Browse… · Chat·send (invokes the model). Each needs an acceptance test
with real or mocked runner state.

## 5. Bugs fixed this session — 🔧
1. **Audit 401 → whole-shell gate cascade.** Audit's `apiFetch` 401'd with no
   session; the shell's global handler swapped every pane for the sign-in gate.
   Fix: dismissable Back button on accidental-401 gates (`auth-gate.ts` +
   `app-shell.ts` + `main.ts` overlay). Plus "Wreath in" → "Login".
2. **Processes · Daemons crash** — `daemons.map is not a function`.
3. **Processes · Processes tab froze on "Loading…"** — same class; the throwing
   render left the prior (loading) frame on screen.
   Root for 2+3: `ProcessApi.listDaemons/listProcesses` declared `Promise<…[]>`
   but returned null/`{}` for empty. Fixed at the api layer (coerce to array) —
   one fix, all list consumers.

**Headline class for the suite:** any binding/list method whose return type is an
array MUST resolve to an array; a non-array makes the consumer's `.length`/`.map`
throw and freezes or blanks the pane. Test every list-fed component against
`null` / `{}` / `undefined` responses.

## 6. Remaining to walk — ❓
Settings section-nav · standalone `?surface=` windows (welcome, about, tray, git,
build, lint, containers, repos, php, marketplace, editor, plugin, ml-lab, lemma,
vi-sites, vi-activity) · the ⚠️ live actions above · empty-state vs populated-state
for every pane (this walk was a fresh/empty install — populated state untested).

## 7. Test approach (no GUI running)
- **vitest + jsdom component tests** for §1–3, §5: mount element, assert render,
  assert empty-state + error-state degrade gracefully, assert list-fed components
  survive non-array responses (the §5 class).
- **bridge-driven** (this skill) for §4 live actions that need a real runner.
