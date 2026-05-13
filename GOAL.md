<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GOAL — lthn-desktop next-arc work

**Working dir:** `/Users/snider/Code/lthn/desktop`
**Branch:** `main`
**Reference:** `plans/project/lthn/desktop/RFC.first-release.md §9` for the omlx parity table.

> Use TDD. Test UI with `cd frontend && bun run test` (vitest + happy-dom; see `frontend/src/lit/chrome.test.ts` for the canonical pattern). Use the AX `_Good` / `_Bad` / `_Ugly` triplet pattern for Go tests. Keep codecov over 70%.

## Codex housekeeping

- Do NOT push. Commit locally; Snider reviews + pushes in the morning.
- Do NOT run `wails3 dev` / `wails3 build` / launch the `lthn` binary — compile-only iterations are 10× faster.
- Iteration loop: `go vet ./go/...` + `go build ./go/cmd/lthn` + `go test ./go/...` + `cd frontend && bun run build` + `bun run test`.
- After any new method on a Wails-exported `*Service`: `cd go && wails3 generate bindings -ts -d ../frontend/bindings -clean=true ./pkg/desktop/...`.
- NO `replace` directives in any `go.mod`. Workspace mode + `external/<dep>` submodules handle resolution.
- NO `git submodule update --recursive` / `foreach --recursive` (leaves orphan locks). Name the submodule path explicitly.
- Audit script line 1 has a stray `yea` token — known quirk, ignore the shell warning.
- Network creds (HF, GitHub push, etc.) are NOT available. Any task needing them: `TODO(snider)` and move on.
- Single commit per logical change, conventional prefix (`feat:` / `fix:` / `test:` / `chore:` / `docs:`). NO commit-spam batching.
- Externals are real clones of `github.com/dappcore/<name>` on `dev`. Commit upstream-side fixes inside `external/<name>/`; Snider pushes upstream later.

---

## Already done (2026-05-13 / 2026-05-14)

- v0.9.0 audit COMPLIANT — codex completed audit phases 1-5 + frontend test hardening, verdict reads `COMPLIANT`.
- Bridge port — 76 tools live across layout/webview/window/screen/file/process/clipboard/lang/theme/focus_set. `pkg/bridge/` directly against Wails alpha.91 + CoreGO primitives (no core/gui dependency).
- Plugin host — Phase 1+2+3+4 shipped (lifecycle + reverse-proxy + crash supervision + tray menu + iframe-mount plugin windows).
- 9 IDE surfaces ported from core/ide — editor, git, build, lint, containers, repos, php, marketplace, plugin.
- RFC.vi-pm.md spec committed in plans tree — Vi-as-PM persona + Lemma training shape.
- core/gui wails alpha.91 upgrade shipped on `codex/gui-wails91-upgrade` branch (pushed to both remotes; main merge pending review).
- core/go-container — 17-task Apple provider port complete on `dev`, pushed to github (DeepSeek run).

---

## Easier-life adds (small wins, do these first)

- AUTH 401 unlock UX — install a `window.fetch` wrapper in `frontend/src/lit/index.ts` that catches 401 on `/v1/*`, surfaces an "Unlock / Setup API key" card in the chrome footer (`frontend/src/lit/chrome.ts`) linking to `settings → API Key`. THEN re-enable `coreapi.WithBearerAuth(opts.LocalKey)` in `go/pkg/server/service.go` (currently TEMP-DISABLED with a `_ = opts.LocalKey` line).
- Window-position auto-save hook — register a `WindowEventClosing` hook on the lthn app that calls `bridge.toolLayoutSave("__autosave")` before quit, and call `bridge.toolLayoutRestore("__autosave")` after `preCreateWindows()` on next boot. ~20 LOC in `go/pkg/desktop/desktop.go`. Closes the "Lethean Desktop remembers my window positions" loop.
- Phase 7 UI wiring — the 45 unwired buttons enumerated in last night's GOAL still aren't wired. Codex got the audit done but Phase 7 wasn't worked. The full bullet list: chat-window (9 items), logs-window (5), benchmark-window (5), distillation-window (4), fleet-window (4), network-window (4), tools-window (4), model-browser-window (5), welcome-window (3), app-shell (2). For each: route `@click` to either the matching Wails binding or to a `TODO(snider): <missing-backend>` comment if no binding exists yet.
- Skill bootstrap — `~/.claude/skills/lthn-bridge/` doesn't exist as a usable path; the skill lives at `/Users/snider/Code/host-uk/core/.claude/skills/lthn-bridge/`. Run `bash /Users/snider/Code/host-uk/core/factory/scripts/bootstrap-cladius.sh` to merge it into the user-claude tree.
- Bridge security tightening — `pkg/bridge/` currently has `Access-Control-Allow-Origin: *` (browser-tab attackable while loopback-bound). Three layered fixes: (1) reflect Origin if loopback, (2) `Host:` header check rejecting non-loopback values, (3) per-launch bearer token at `~/Lethean/conf/bridge-token` (mode 0600) required on every request. ~80 LOC across `bridge.go` + `tools.go`. Mirror the same tightenings into `core/ide/go/pkg/server/mcp_bridge.go`.
- Build-tag separation — add `//go:build bridge` to every file in `pkg/bridge/`, create a `bridge_stub.go` with `//go:build !bridge` returning an inert service so `app.go` compiles unchanged. Taskfile.yml `dev` task gets `-tags bridge`; `build` task doesn't. Production binary ships with zero bridge attack surface.
- TODO(snider) reverse-index — run `grep -rn "TODO(snider)" frontend/src/ go/pkg/` and write the results to `docs/snider-backlog.md` so the marker doesn't get buried in code.
- GOAL-STATUS.md cleanup — codex/deepseek leave these after runs. Keep as audit trail in repo; future-Snider greps them for context. No action needed unless one shows up in a public-facing surface.
- Dependabot triage — `LetheanNetwork/desktop` reports 10 vulnerabilities (1 critical, 2 high, 5 moderate, 2 low). Visit `https://github.com/LetheanNetwork/desktop/security/dependabot`, classify each by whether it's actually in our hot path (most won't be — they'll be transitive deps in `node_modules`). One commit per fix; suppress out-of-hot-path ones with explanation.

---

## v0.2 — OAI/Anthropic parity + auto-update + supervision

- runner.Service: `/v1/chat/completions` SSE-stream wiring on the coreapi engine (already routed; verify it actually streams to OAI clients)
- runner.Service: `/v1/completions` legacy endpoint (transformer over chat-completions)
- runner.Service: `/v1/messages` Anthropic-compatible (coreapi has the TransformerIn/Out pattern — write the Anthropic transformer)
- runner.Service: `/v1/models` enumeration of loaded models with metadata (size, family, quant, ctx length)
- AUTH 401 unlock UX — fetch wrapper catches 401 and opens settings → API Key (gated on apikey.Reveal returning a value)
- Auto-update — Sparkle integration (macOS) checking forge.lthn.sh releases manifest
- Auto-restart on crash — `lthn-supervisor` separate binary, same shape as the plugin host but for the lthn binary itself
- Welcome onboarding — 3-step flow already in welcome-window.ts; needs Step 1 folder picker + Step 2 model picker + Step 3 client wiring (cross-refs Phase 7 GOAL items)

## v0.3 — Multi-model + tool calling + MCP + downloader

- runner.Service: multi-model LRU with per-model TTL + pinning (cross-machine state primitive supplies the cold tier)
- runner.Service: continuous batching via `dappco.re/go/inference` scheduler
- runner.Service: speculative decoding via go-mlx decode primitive
- runner.Service: tool calling — Llama / Qwen / Gemma / MiniMax / Mistral / Kimi / Longcat parsers (each lives in `dappco.re/go/inference` once landed)
- runner.Service: Claude Code optimisation — context-scaling + SSE keep-alive defaults
- runner.Service: HuggingFace mirror endpoint (`/v1/models/<id>/resolve/<file>` proxying to HF with on-disk cache)
- tools.Service: `Invoke(name, args)` — wire via mcp.Server's typed handlers
- tools.Service: `RegisterServer(spec)` — `mcp.Options.Subsystems` accepts pluggable Subsystem at runtime; surface via Wails
- tools.Service: `SetEnabled(id, bool)` — toggle a registered server
- models.Service: `Download(id, dest)` — fetch via `dappco.re/go/mlx/hf.RemoteSource` with progress events streamed via SSE
- models.Service: `Import(srcPath)` — copy local .gguf into models dir using `gguf.Info` parser to validate
- models.Service: `Delete(id)` + `Pin(id, bool)` — file-system + config-backed
- Per-model settings window — surface per-model defaults (temp, top_p, ctx, system prompt)

## v0.4 — Embeddings + web admin + benchmark + i18n + integrations + CUDA

- runner.Service: `/v1/embeddings` (BERT / BGE-M3 / ModernBERT support via go-mlx embeddings)
- runner.Service: `/v1/rerank` (cross-encoder rerankers)
- runner.Service: `Bench(opts)` — PP / TG / Both benchmark mode against the loaded model, report to benchmark-window.ts
- web admin window — full surface inside a transient native window (chat / models / settings / integrations / logs)
- benchmark-window.ts: already wireable once `runner.Bench` lands (Phase 7 item 7.3.2)
- i18n parity — every Lit window honours `c.I18n().Translate()`; fr / en_au / zh already exist, add tooling for community contributions
- integrations.Service: `SetEnabled(clientID, bool)` — wire/unwire OpenClaw, Codex, Cursor, Claude Code (each writes the OAI-compat config to that client's expected path)
- integrations.Service: `WriteConfig(clientID)` — emit the per-client config file
- CUDA backend — `go-mlx` Metal already shipped; CUDA via cgo to libcuda OR via go-mlx's runtime selector

## v0.5 — VLM + heterogeneous multi-card

- runner.Service: vision-language model support — Qwen3.5-VL, GLM-4V, Pixtral via go-mlx VLM bindings
- Vi tool-call: `vi.look(image)` — Vi loses native vision in training (per RFC.vi-pm.md §3.4), uses this tool when she needs to see something; routes to whichever VLM is loaded
- runner.Service: heterogeneous multi-card "logical flow" — Apple Metal + AMD HIP + CUDA in same process, model layers distributed by hardware fit
- Decision rule: which layer goes where is read from a config-table or auto-tuned via a one-shot calibration run on first launch
- LARQL layer-portability: each transformer layer's weights serialised in a format the runner can ship between cards at runtime

## v0.6 — OCR

- runner.Service: OCR endpoint — DeepSeek-OCR / DOTS-OCR / GLM-OCR via go-mlx OCR bindings
- Vi tool-call: `vi.read_document(path)` — Vi can OCR a document and reason over its contents

## v0.7 — P2P + homelab compute + AMD card

- p2p.Service — wraps `dappco.re/go/p2p.node.Service` (already in workspace candidate per RFC.first-release.md)
- p2p.Service: `Pair(homelab-pubkey)` — first-time pairing handshake establishing trust between the user's lthn-desktop and the user's homelab node
- p2p.Service: `Discover()` — local network discovery (mDNS / IPv9 / LetherNet routing)
- p2p.Service: `Status()` — connected peers, latency, throughput, hardware fingerprint (Apple Metal / AMD HIP / CUDA / CPU)
- p2p.Service: `Capabilities(peer)` — RPC asking a peer what models / cards / accelerators it has
- runner.Service: peer-routed inference — when a model would fit better on the homelab's AMD card, route the request there transparently, stream tokens back over the p2p tunnel
- runner.Service: peer-routed fallback — when local OOM, automatically retry on a paired peer
- network-window.ts: surface paired peers, their hardware, current routing decisions (Phase 7 items 7.6.x become real here)
- fleet.Service — multi-machine *controller*; routes inference + work units across the user's owned hardware (homelab + laptop + studio + iPad-with-companion)
- fleet-window.ts: machines / routing rules / snapshots tabs wire up (Phase 7 items 7.5.x become real here)
- AMD HIP backend on the homelab side — go-mlx HIP build target, runs on the homelab's AMD card, exposed via p2p as a routable inference peer

## v0.8 — Sandboxed yolo agent (Docker first, TIM later)

### v0.8-a — easy route: load the core-dev OCI image on whichever runtime is present

- The IMAGE is `core-dev:latest` (OCI artefact, 5.68 GB, 65+ linters + security scanners + SBOM tools, built 2026-03-30 per CLAUDE.md). The RUNTIME is whichever container engine `pkg/container/` (shipped 2026-05-13) probes as available on this host — Docker, Podman, Apple Container, LinuxKit. When we say "Docker", we mean the image format; not the daemon.
- pkg/sandbox — picks the highest-priority available runtime from `container.Service.Detect()`, dispatches to it via process.Service
- pkg/sandbox: `Spawn(opts)` — runs core-dev with a defined resource budget (CPU / RAM / disk / GPU passthrough optional) using the chosen runtime's native command (Docker → `docker run`, Podman → `podman run`, Apple Container → `container run`)
- pkg/sandbox: agent profile — drops an `agent` binary into the running container in `--yolo` mode (unrestricted within the container)
- pkg/sandbox: `/v1/api/sandbox/<id>/*` reverse-proxy mount — same shape as the plugin host, agent inside serves an HTTP surface lthn can route to
- pkg/sandbox: filesystem boundary — mount `~/Lethean/data/sandboxes/<id>/` as the container's only writable bind, everything else is the image's read-only rootfs
- pkg/sandbox: network boundary — `--network=none` (or runtime-equivalent) by default, override only when the use case explicitly needs internet (deep-research)
- pkg/sandbox: tool-call gating — agent inside calls lthn over a UNIX socket bind-mounted into the container; lthn enforces policy at the gate
- Runtime-agnostic by construction: same lthn API surface regardless of which runtime is supplying the isolation. User installs Docker, Podman, Apple Container — whichever — and lthn finds it and uses it.
- This is the working substrate. Good enough for real use while v0.8-b lands.

### v0.8-b — proper polish: LinuxKit immutable + STIM bundles

- pkg/sandbox: swap the Docker runtime for `dappco.re/go/container` (LinuxKit + TIM) once we have time to craft the immutable image properly — no rush, no deadline
- pkg/sandbox: STIM bundle support — encrypted-at-rest agent snapshots via Borg (memory + filesystem state encrypted under user's key, replay anywhere with the key)
- pkg/sandbox: hardware isolation — TIM's distroless minimal-attack-surface image vs the kitchen-sink core-dev image; the contract above stays identical, only the runtime swaps

### Either runtime — what we get

- A `--yolo` agent inside the sandbox is **structurally** safe — the agent can write any code, run any command, fetch any URL, install any package — and none of that touches the host. The sandbox is the moat.
- Use case 1: agent-driven development — "agent, build me a Laravel app that does X" runs entirely inside the sandbox; finished artefact is exported back to the user's project tree on completion
- Use case 2: dependency audit — drop an unknown binary into a sandbox, let the yolo agent poke at it, report findings; binary never touches the host
- Use case 3: deep-research agent — gives the agent unrestricted internet but no host access; output is a structured report

## v1.0 — External overflow + sovereign auth + commercial polish

- runner.Service: external API overflow via `dappco.re/go/ratelimit` — when local + peer-routed compute exhausts, fall over to a configured external provider (OpenAI / Anthropic / OpenRouter) with the user's own keys, rate-limited per their budget
- pkg/auth — full PGP / LetheanAccount handshake replacing API-key auth (per chat with Snider: server.key created at first boot locked with `lthnHash(CWD)`, client sends LetheanAccount payload encrypted under server pubkey, tunnel established, service-handle routing) — see RFC.vi-pm.md §3 cross-reference
- Distribution polish — signed + notarised macOS .app, Windows MSIX, Linux .deb/.rpm/.AppImage, all auto-update
- Telemetry opt-in — anonymised usage stats land at a public dashboard so the community sees real adoption numbers
- Vi-Lemma model bundling — `vi-lemma-<version>.gguf` downloaded on first launch (or shipped in installer for offline distribution)

---

## Lthn-exclusive surfaces (not on the omlx parity ladder)

- plugin host runtime — shipped 2026-05-13; lthn-exclusive
- 76-tool agent bridge — shipped 2026-05-13; lthn-exclusive (`pkg/bridge/`)
- Per-surface windows that remember positions — `layout_save` + `layout_restore` shipped via bridge; auto-save hook listed in Easier-life adds
- Vi-as-PM conversation layer — RFC.vi-pm.md scope, ships alongside v0.3+ once the engineering pipeline is fast enough to feed Vi's review flow
- Vi knowledge embed — `pkg/vi/embed.go` with `//go:embed all:knowledge` mirroring core/agent .md library so Vi has team vocabulary in context at boot. Build-time script to copy core/agent docs into `pkg/vi/knowledge/`.
- Sovereign-account auth (LetheanAccount) — v1.0 row, no omlx counterpart
- State primitive (KV-as-video, cross-machine portable) — substrate already in go-mlx, surface lands progressively across releases

---

## Add to RFC.first-release.md §9

When this GOAL.md graduates to active work, port the **lthn-exclusive** rows back into §9.2 as new table rows:

| Row to add | Property | v |
|---|---|---|
| Plugin runtime | OOB-binary + reverse-proxy mount, extensible at runtime | v0 (lthn-exclusive, shipped) |
| Agent bridge | 76 tools — webview drive, window control, layout memory, file/process/clipboard | v0 (lthn-exclusive, shipped) |
| Per-window position memory | layout_save / layout_restore via bridge + autosave hook | v0 (shipped, autosave pending) |
| Vi-as-PM | Model-as-PM mediating review flow on user-owned hardware | v0.3 |
| TIM-isolated yolo agent | Distroless Linux + Borg-encrypted bundle for unrestricted agent work | v0.8 |
| Sovereign auth (LetheanAccount) | PGP-keyed account flow, server.key locked to CWD | v1.0 |

---

## Notes

- The mapping discipline: every bullet here links back to either an omlx parity row or a lthn-exclusive property. Bullets that drift outside that frame should be relocated to a sibling RFC (Vi → RFC.vi-pm, auth → its own RFC, plugin → already shipped).
- The release ordering is not a schedule. v0.2 ships when v0.2 is done; v0.5 ships when v0.5 is done. Each release is a feature commitment, not a date.
- TIM bundles are the security boundary that lets the rest of the agent-driven pipeline be reckless. The yolo agent CAN be reckless because the bundle is the wall. Same shape as the plugin host's reverse-proxy boundary, escalated to the OS-process level.
- Each Easier-life add is small + reviewable on its own. Sequence them: AUTH unlock UX → auto-save hook → Phase 7 UI wiring → skill bootstrap → bridge security tightening → build-tag separation → TODO reverse-index → Dependabot triage. Each is a separate commit.
