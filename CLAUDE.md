<!-- SPDX-License-Identifier: EUPL-1.2 -->

# CLAUDE.md — Lethean Desktop

Read [`AGENTS.md`](AGENTS.md) before changing this repository. It is the
canonical product, architecture, CoreGO, security, development, and
verification contract. This file is intentionally short so a second agent
guide cannot drift into a competing repository description.

## Product identity

This repository builds the `lthn` CLI router and its Wails-hosted Angular
application. The GUI is one consumer of the same Go composition used by CLI
and server commands; a frontend failure must not silently break non-GUI
commands.

Canonical paths:

- `go/cmd/lthn/` — CLI router, composition root, and embedded production
  assets.
- `go/pkg/` — product services.
- `frontend/` — the only product frontend; Angular, standalone, CSR, and
  hash-routed.
- `frontend/bindings/` — ignored generated Wails TypeScript bindings.
- `frontend/` — two Wails mobile support files, not another application.
- `docs/design/lit/` — visual reference material, not a build input.

Dependencies resolve from `go/go.mod`, `go/go.sum`, `go.work`,
`frontend/package.json`, and `frontend/package-lock.json`. There are no
required submodules or `external/` source checkouts.

## Start here

```bash
go tool wails3 task doctor

# Browser-only deterministic design/demo mode
cd frontend
npm ci
npm run demo

# Full native development with Angular HMR
cd ..
go tool wails3 task dev
```

`go tool wails3 task dev` uses Angular HMR on 9245, Wails MCP on 9099, and the Lethean
binding transport on 9199. Production builds remain embedded under the
`wails://` asset route.

## Confidence gates

```bash
go tool wails3 task verify:frontend
go tool wails3 task test
go vet ./go/...
cd frontend && npm run build
```

Run `bash build/audit.sh` for the broader no-regression diagnostic plus
blocking build and test checks. The external CoreGO v0.9.0 compliance result
has a known historical backlog and is not an all-zero repository gate.

## Non-negotiable boundaries

- Use British English and EUPL-1.2.
- Do not restore the retired Lit application, Bun workflow, submodules,
  Angular SSR, hydration, or another product frontend.
- Keep runtime Lit where it is intentional: `frontend/src/kit/` and
  plugin descriptors whose `kind` is `lit`.
- Every file-backed product operation must ultimately flow through a
  registered `dappco.re/go/io.Medium`.
- Preserve mobile support files and design references unless their consumers
  and tests are deliberately migrated in the same change.
- Use TDD for behaviour and the repository's focused Good/Bad/Ugly CoreGO
  conventions.
