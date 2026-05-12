---
title: lthn
description: Lethean Desktop binary — a CLI router that dispatches to a tray GUI, an OpenAI-compatible HTTP server, an AI runtime, and future blockchain / LNS / wallet modules. Sovereign-compute local AI on user-owned hardware.
---

<!-- SPDX-License-Identifier: EUPL-1.2 -->

# lthn

**Binary:** `lthn`
**Module path:** `dappco.re/lthn/desktop`
**Language:** Go 1.26 + Lit (frontend)
**Licence:** EUPL-1.2

lthn is the Lethean Desktop binary. It is a single executable that composes multiple Lethean subsystems behind one CLI:

- A native macOS tray + popover GUI (Wails) for interactive use.
- An OpenAI-compatible HTTP server for clients (Claude Code, OpenCode, Codex, etc.).
- An AI runtime built on `go-mlx` (Apple Metal native) for local inference.
- Future modules side-loaded into the same binary: blockchain controls, decentralised TLD (`.lthn`) name resolution, wallet operations.

The product story is **sovereign compute, single-watt**: local LLM inference on user-owned hardware, no cloud round-trip, airplane-mode capable, with the on-disk state visible to the user (no hidden dot-dirs) and encrypted under the user's workspace key.

---

## The CLI is the binary's identity

`lthn` is a CLI router first. Each subcommand maps to a subsystem:

```
lthn                        # default — launches tray + GUI when wired
lthn version                # version info
lthn gui                    # explicit Wails launch (tray + popover + windows)
lthn tray                   # tray-only mode (NSStatusItem, no popover pre-open)
lthn serve [--port PORT]    # HTTP API only (OpenAI-compatible)
lthn ai chat                # interactive REPL with the loaded model
lthn ai generate "prompt"   # one-shot generation
lthn ai models ls           # list local models in ~/Lethean/conf/models/
lthn ai models pull NAME    # download from HuggingFace
lthn help [subcommand]      # built-in help
```

Future subcommands per the `lthn` unified namespace canon (binary / CLI / URI / DNS / LNS / Core Action namespace — see `plans/project/lthn/RFC.md` §7):

```
lthn gateway vpn ...        # gateway / VPN controls (when wired)
lthn build ...              # branded build pipeline
lthn wallet ...             # blockchain wallet (when side-loaded)
```

`lthn://` URI handlers route through the same dispatch.

---

## Architectural rule

**The Wails GUI is decoupled from the deliverable.** The binary's identity is the CLI router; the GUI is one consumer of that dispatch. If the GUI build is broken, `lthn serve` and `lthn ai` still ship and work. New surfaces are added as additional subcommands and additional `pkg/*/service.go` modules — never as forks of the binary entry.

When the tray runs (`lthn` default mode or `lthn gui`), the NSStatusItem is the lifetime anchor: closing all windows does NOT quit the app. Windows are transient surfaces anchored to the tray-process.

---

## User-data convention

lthn (and every Lethean app) writes user data under `~/Lethean/` — visible in Finder, never hidden in dot-dirs. The conventional sub-layout:

```
~/Lethean/
├── cli/         binaries (lthn, letheand, lethean-wallet-*, etc.)
├── data/        runtime data (logs, lmdb, generated artefacts)
├── conf/        configuration
│   ├── models/  AI models (consumed by lthn ai)
│   └── keys/    signing keys, codesigning material
└── wallets/     user wallet files (when blockchain side-loads)
```

The "no hidden user bloat" rule is structural: standard "drag .app to trash" uninstall must leave nothing significant behind. See `plans/project/lthn/desktop/RFC.first-release.md` §7 for the resolution.

---

## Reading next

- `docs/architecture.md` — how the dispatch + subsystems compose
- `docs/development.md` — how to build, run, test, audit
- `AGENTS.md` — repo-local notes for code-writing agents
- `plans/project/lthn/desktop/RFC.first-release.md` — first-release scope (P0 surface + v0 to v1.0 trajectory)
- `plans/project/lthn/desktop/DESIGN-BRIEF.md` — design canon references (Lethean-4 visual + Lethean-5 Lit)
- `plans/project/lthn/RFC.md` §7 — the `lthn` unified namespace canon
