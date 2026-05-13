<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Plugin host — scope

**Status:** draft scope · 2026-05-13
**Package target:** `pkg/plugin/`
**Unblocks:** marketplace Install/Remove · CoreAgent first ship · "default features are just bundled plugins" framing

---

## Why this exists

`lthn` today ships a fixed feature set inside one binary. The architecture Snider
articulated for the next stage is:

> *"start binary on a port, proxy bind as a gin v1/api/plugin/, on the binary on
> a random port you add a namespace prefix to its routing… so once T3 is in,
> the software inside lthn-desktop become 'default' features, and we add more
> with Plugins, our first being CoreAgent."*

The Snider/Miner binary already proves the pattern — `--namespace` CLI arg is
the load-bearing detail. This scope formalises lthn's side: the host that
**finds, fetches, starts, supervises, and reverse-proxies** plugin binaries.

The marketplace surface that landed in commit `568d01f` returns "plugin host
not running" from Install/Remove today. This work makes those calls real.

---

## Architecture

```
┌──────────────────────── lthn (the host binary) ─────────────────────────┐
│                                                                          │
│   coreapi.Engine ─── /v1/api/plugin/<ns>/* ──┐                          │
│                                              │ reverse-proxy            │
│                                              ▼                          │
│   pkg/plugin.Service                ┌────────────────┐                  │
│     ├─ Install/Remove (disk + spawn)│  127.0.0.1:<n> │ ←── ManagedProcess │
│     ├─ Start/Stop      (process)    │  <plugin bin>  │     (process.Service)│
│     ├─ List/Status     (in-memory)  │  --namespace=X │                  │
│     └─ Proxy mount     (coreapi)    │  --port=<n>    │                  │
│                                     │  --token=<k>   │                  │
│                                     └────────────────┘                  │
└──────────────────────────────────────────────────────────────────────────┘
```

The plugin host is a **Wails-bindable service** in `pkg/plugin/` that owns:

1. **Disk** — `~/Lethean/conf/plugins/<code>/{plugin.json, bin/<exe>, data/}`
2. **Process** — N ManagedProcess instances supervised via `dappco.re/go/process`
3. **Routing** — N reverse-proxy mounts on the `coreapi.Engine` at
   `/v1/api/plugin/<code>/*`
4. **Catalogue link** — knows about the marketplace fixture entries; Install
   resolves a fixture record to a binary URL + manifest

It does **not** own auth (bearer token comes from `pkg/apikey`), inference
(plugins talk to lthn's OpenAI-compatible API like any other client), or
sandboxing (v1 plugins run as the user — sandboxing lands when
`project_display_server_native_agent_sandbox.md` does).

---

## Plugin contract

A plugin is **any binary** that obeys this CLI contract:

```
plugin-bin --namespace=<ns> --port=<int> --token=<api-key> [--data=<dir>]
```

| Flag          | Owner | Meaning                                                   |
|---------------|-------|-----------------------------------------------------------|
| `--namespace` | host  | URL slug under `/v1/api/plugin/<ns>/`. Plugin mounts its routes under this prefix internally so the reverse-proxy doesn't have to rewrite paths. |
| `--port`      | host  | TCP port the plugin must bind on `127.0.0.1`. Host picks via `net.Listen(":0")` then passes the port number, avoiding any race / stdout-parsing dance. |
| `--token`     | host  | The current local API key (from `pkg/apikey`). Plugin uses this for any calls back into lthn's HTTP surface. Rotates when the user clicks Rotate in the UI. |
| `--data`      | host  | Plugin's writable data dir (`~/Lethean/conf/plugins/<code>/data/`). Optional — plugins that need no on-disk state can ignore. |

**Plugin responsibilities:**

- Bind to `127.0.0.1:<port>` only — no listening on 0.0.0.0, no other ports
- Mount routes under `/<namespace>/*` (or accept that the host strips that
  prefix on the way through — open question, see §Open questions)
- Implement `GET /<namespace>/health` returning HTTP 200 within 5 s of start
- Honour SIGTERM gracefully (10 s drain window before SIGKILL)

**What the host gives back via the reverse-proxy:**

- `X-Lthn-User` header on every forwarded request (the user's stable id —
  useful when plugins want to keyspace their state per user)
- `Authorization: Bearer <token>` forwarded transparently when the calling
  client already sent a bearer (no token escalation — plugin gets only what
  the caller had)
- Streaming preserved (SSE / WebSocket upgrade pass through unchanged)

---

## Manifest format — `plugin.json`

Stored in `~/Lethean/conf/plugins/<code>/plugin.json` after Install:

```json
{
  "code":        "coreagent",
  "name":        "CoreAgent",
  "version":     "0.1.0",
  "namespace":   "coreagent",
  "binary":      "bin/coreagent",
  "permissions": ["network", "config-read"],
  "menu": {
    "label":   "CoreAgent",
    "icon":    "fa-robot",
    "surface": "coreagent"
  },
  "health": {
    "path":     "/coreagent/health",
    "interval": 30,
    "timeout":  5
  },
  "ui": {
    "entrypoint": "/coreagent/ui/",
    "embed":      "iframe"
  }
}
```

| Field         | Required | Meaning                                                |
|---------------|----------|--------------------------------------------------------|
| `code`        | yes      | Stable identifier. Matches marketplace catalogue key.  |
| `namespace`   | yes      | URL slug. Usually `code` but can differ if needed.     |
| `binary`      | yes      | Relative path inside the plugin dir to the executable. |
| `permissions` | no       | v1 informational only — surfaced in the UI but not enforced. v2 wires real capability gates. |
| `menu`        | no       | When present, the host adds a menu entry to the lthn tray + a sidebar entry in the GUI. `surface` becomes a routable `?surface=plugin-<code>` value. |
| `health`      | no       | Defaults to `/<namespace>/health` with 30 s interval, 5 s timeout. |
| `ui`          | no       | When present, the marketplace gets a "Open" button that opens an embedded view at `/v1/api/plugin/<code>/<ui.entrypoint>`. |

Manifest lives in the catalogue too — when the host resolves a marketplace
`Install(code)` call, it fetches the manifest from the catalogue + the binary
from the manifest's `repo` field (a release artefact URL).

---

## Lifecycle

### Install(code)

1. Resolve `code` against the marketplace fixture (or remote registry once
   wired). Get the manifest + binary URL.
2. Verify the binary URL is HTTPS + the host matches an allowlist
   (forge.lthn.sh / github.com/dappcore — configurable).
3. Download the binary to `~/Lethean/conf/plugins/<code>/bin/<exe>` with a
   bounded-size limit (32 MB v1 default).
4. Verify SHA-256 against the manifest's checksum (when present).
5. `chmod +x` the binary. On macOS, run `xattr -d com.apple.quarantine` to
   clear Gatekeeper unless the binary is signed.
6. Write `plugin.json` to the plugin dir.
7. Call `Start(code)` so the plugin is immediately running.

### Start(code)

1. Read `plugin.json` from disk.
2. Pick a free port: `l, _ := net.Listen("tcp", "127.0.0.1:0"); port := l.Addr().(*net.TCPAddr).Port; l.Close()`. *(Yes there's a TOCTOU race; in practice it doesn't bite at the scales we care about. v2 can use systemd-style socket activation if needed.)*
3. Spawn the binary via `process.Service.StartWithOptions` with:
   ```
   command: <plugin-dir>/bin/<exe>
   args:    [--namespace=<ns>, --port=<n>, --token=<k>, --data=<plugin-dir>/data]
   dir:     <plugin-dir>
   env:     [LTHN_PLUGIN=1, LTHN_HOST=http://127.0.0.1:<lthn-port>]
   ```
4. Wait for the health endpoint to return 200 (up to 5 s).
5. Register the reverse-proxy mount on `coreapi.Engine`:
   ```go
   engine.Register(&pluginRoutes{
       code: code,
       ns:   ns,
       proxy: httputil.NewSingleHostReverseProxy(&url.URL{
           Scheme: "http",
           Host:   "127.0.0.1:" + strconv.Itoa(port),
       }),
   })
   ```
6. Update the in-memory `installedState[code]` to `Running{Port: port, PID: pid, StartedAt: now}`.

### Stop(code)

1. Send SIGTERM to the managed process. Wait up to 10 s.
2. SIGKILL on timeout.
3. Unregister the reverse-proxy mount.
4. Update state to `Stopped{StoppedAt: now}`.

### Remove(code)

1. `Stop(code)` if running.
2. `rm -rf ~/Lethean/conf/plugins/<code>/`.
3. Drop from `installedState`.

### Supervision

The host watches each `ManagedProcess.Done()` channel. On unexpected exit:

- If `Status == StatusCrashed` and crash count < 3 in last 60 s: restart with
  100 ms × 2^n backoff.
- After 3 crashes: mark `Dead{LastError: ...}`, do not restart, surface in UI.
- On clean SIGTERM exit (user-initiated `Stop`): no restart.

---

## Wails surface

Bound on `*Service`, generated to `frontend/bindings/.../pkg/plugin/service`:

```go
func (s *Service) Install(code string) (InstallOutput, error)
func (s *Service) Remove(code string) error
func (s *Service) Start(code string) (Status, error)
func (s *Service) Stop(code string) error
func (s *Service) List() (ListOutput, error)       // installed + state
func (s *Service) Status(code string) (Status, error)
```

The marketplace package today returns "plugin host not running" from
Install/Remove. After this lands, marketplace becomes a thin caller:

```go
func (m *marketplace.Service) Install(code string) error {
    if ph, ok := core.ServiceFor[*plugin.Service](m.core, "plugin"); ok && ph != nil {
        _, err := ph.Install(code)
        return err
    }
    return core.E(...)
}
```

---

## Out of scope for v1

| Item                              | Why deferred                                |
|-----------------------------------|---------------------------------------------|
| Sandboxing / capability enforcement | Project memory `display_server_native_agent_sandbox.md` covers the OS-level isolation work. Wires in once that lands. |
| Cross-machine plugins             | Plugins are local processes only. Fleet/community-compute is a separate axis. |
| Plugin-to-plugin direct calls     | Plugins talk to each other via lthn's HTTP surface (`/v1/api/...`). No private IPC channel v1. |
| Hot-reload of running plugins     | Stop → upgrade → Start is fine v1.          |
| Multiple instances of the same plugin | `code` is a singleton key v1.            |
| GUI for permissions               | Manifest's `permissions` list is informational only; surfaced read-only. |
| Auto-update                       | User clicks Install with the new version; old binary replaced after Stop. |
| Plugin signature verification     | SHA-256 only v1; cert-pinned signature in v2. |

---

## Package shape

```
pkg/plugin/
├── plugin.go      — Service, types (Manifest, Status, InstalledPlugin)
├── manifest.go    — JSON parsing + validation
├── disk.go        — install dir layout, atomic write helpers
├── fetch.go       — HTTPS download with size/host allowlist
├── supervisor.go  — ManagedProcess wiring, crash-loop logic
├── proxy.go       — coreapi.RouteGroup that mounts the reverse-proxy
├── wails.go       — Wails-bindable methods (Install/Start/Stop/...)
└── plugin_test.go — table-driven coverage of lifecycle paths
```

Approximate size: 600–900 LOC including tests. The `proxy.go` is the only
load-bearing new abstraction; everything else is plumbing over already-built
primitives (`process.Service`, `coreapi.Engine`, `core.PathJoin`).

---

## CoreAgent as the canonical first plugin

CoreAgent ships as `lthn/coreagent` — a separate Go binary that:

1. Accepts the plugin CLI contract above.
2. Mounts a gin router that includes:
   - `/coreagent/health`
   - `/coreagent/api/*` — REST surface (chat, brain, sessions, tools)
   - `/coreagent/mcp/*` — MCP tool registry (via `coreapi.ToolBridge`)
   - `/coreagent/ui/*` — embedded Lit UI bundle (its own dist/)
3. Talks back to lthn for inference via `LTHN_HOST` env var + the
   `LTHN_TOKEN` argument — exactly like any third-party SDK would.
4. Stores state in `--data` (passed by the host) so a `Remove(coreagent)`
   takes the data with it.

Once CoreAgent runs as a plugin, the "default features" framing materialises:
chat, brain, sessions, tools today live in `pkg/runner` / `pkg/bridge` /
`pkg/sessions` etc. inside lthn-host. They keep working as default-installed
plugins or get migrated into CoreAgent — that's a separate migration arc
once the runtime is proven.

---

## Open questions

1. **Path stripping at the proxy boundary.**
   When the host receives `GET /v1/api/plugin/coreagent/api/chat/completions`,
   does the plugin see `/coreagent/api/chat/completions` (full path preserved)
   or `/api/chat/completions` (namespace prefix stripped)? Recommendation:
   **preserve the full path under `<namespace>/...`** because the plugin
   already knows its namespace (from `--namespace`) and doesn't need the
   host to strip. Symmetric with Miner's existing pattern.

2. **Token rotation propagation.**
   When the user clicks Rotate in apikey, all running plugins suddenly have
   stale tokens. Options: (a) restart all plugins; (b) signal plugins via
   `SIGHUP` + they re-read from a file the host writes; (c) require plugins
   to handle 401 by re-reading from `--data/token.txt`. Recommendation: **(a)
   restart all** — token rotation is rare, restart is reliable, plugins
   stay simple.

3. **Health check during startup.**
   Plugin gets 5 s to return 200 on `/<ns>/health`. What if the plugin needs
   longer (loading a model, etc.)? Recommendation: **manifest declares a
   `startup_timeout`** that overrides the default. CoreAgent likely needs
   30 s to warm up inference.

4. **Binary verification.**
   v1 has SHA-256 against the manifest. The manifest itself comes from where?
   For fixture entries, it's compiled in (trusted). For a remote registry,
   the manifest needs its own signature. Recommendation: **fixture-only v1**;
   real registry signature is a v2 concern once `core/scm/marketplace` lands.

5. **What happens to a plugin's listening port when the host crashes?**
   Plugins are children of the host process — when host dies, plugins get
   SIGHUP (because process group). v1 acceptable: plugins should also die.
   v2: pidfile-based recovery so a host restart re-adopts running plugins.

---

## Decision points needed before implementation

- [ ] **Path stripping** — preserve or strip namespace prefix?
- [ ] **Default `~/Lethean/conf/plugins/` location** — confirms `conf/` is the
  right tier (vs `data/` which holds wallets + caches). Memory says
  conf/models/ for model files; conf/plugins/ is consistent.
- [ ] **Binary hosting** — forge.lthn.sh/releases vs github.com/dappcore
  releases for the canonical hosting source. Fixture allowlist follows.
- [ ] **CoreAgent repo scaffolding** — separate repo at
  `forge.lthn.sh/lthn/coreagent` with its own go.mod / Dockerfile / release
  workflow? Or first iteration as a sibling package inside lthn-desktop
  proven, then split?

---

## Estimated effort

| Phase | Scope | LOC |
|-------|-------|-----|
| 1 — Static lifecycle | Install (local file copy, no fetch yet) + Start + Stop + Remove + reverse-proxy mount + List/Status. Ship a hand-built test plugin that just echoes back. | ~400 |
| 2 — Fetch + manifest | HTTPS download + SHA-256 verify + manifest parse + host allowlist. Marketplace.Install becomes real. | ~200 |
| 3 — Supervision | Crash-loop detection + backoff + Dead state surface. | ~150 |
| 4 — Menu/UI integration | Plugin menu entries surface in the tray + sidebar; `?surface=plugin-<code>` routing in main.ts; iframe-mount UI plugins. | ~250 |
| 5 — CoreAgent | Separate repo + binary + UI bundle + MCP tools + first end-to-end Install demo. | (separate repo) |

Phase 1 is what unblocks marketplace Install. Phases 2-4 layer on. Phase 5
proves the whole stack and feeds back into refinements.
