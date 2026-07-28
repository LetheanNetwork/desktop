<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Frontend Migration Retirement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans
> to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for
> tracking.

**Goal:** Finish retiring the removed Lit/submodule development topology so
every active build path, source comment, and developer entrypoint describes
the Angular 22 and versioned-module repository that actually ships.

**Architecture:** Keep the already-correct binding, CI, and audit
implementations unchanged and prove them through their executable convergence
contracts. Update source comments to point at canonical Angular/generated
locations, consolidate duplicated agent guidance behind `AGENTS.md`, make the
developer guide runnable from a normal clone, then prove the tracked tree in a
disposable detached worktree.

**Tech Stack:** Go 1.26, Wails 3, Task, Angular 22, npm, Node test runner,
Git detached worktrees.

## Global Constraints

- Execute inline on `main`; do not use sub-agents.
- Preserve the user's `go.work.sum` and `.playwright-mcp/` changes.
- `frontend/` remains the only product frontend.
- Keep the two tracked mobile support files under `frontend/`.
- Do not restore `external/`, `.gitmodules`, Bun, SSR, or the retired Lit app.
- Do not change UX or product behaviour in this documentation/tooling tranche.
- Use British English and EUPL-1.2 headers.
- Do not add source-text tests for human prose; verify executable build
  contracts by running them.

---

### Task 1: Reconfirm the active executable topology

**Files:**

- Modify: `Taskfile.yml` comments
- Modify: `build/config.yml` comments
- Verify: `build/Taskfile.yml`
- Verify: `.github/workflows/build.yml`
- Verify: `build/audit.sh`
- Verify: `scripts/verify-frontend-convergence.test.mjs`

**Interfaces:**

- Consumes: the existing `test:contracts` frontend confidence entrypoint.
- Produces: fresh evidence that bindings target `frontend`, CI has no
  submodule checkout, and the audit uses npm under `frontend`.

- [x] **Step 1: Run the focused convergence contracts**

```bash
cd frontend
npm run test:contracts
```

Expected: the binding-generator harness, CI topology, audit topology, mobile
binding restoration, and frontend verification contracts pass.

- [x] **Step 2: Inspect the executable files against the contract**

Confirm:

- `common:generate:bindings` generates only
  `frontend/bindings`;
- no build task references `external/gui`;
- checkout steps use no recursive submodules; and
- `build/audit.sh` runs npm build, test, and contract commands from
  `frontend`.

If the contract and implementation are already green, do not rewrite them.
Correct comment-only scaffold language where it still describes Lit, npm
install, a proxy, or `go mod tidy` as the active development path.

---

### Task 2: Retire stale paths from active Go comments

**Files:**

- Modify: production `.go` files under `go/` which mention
  `frontend/bindings` or `frontend/src/lit`

**Interfaces:**

- Consumes: canonical generated bindings under `frontend/bindings` and
  Angular surfaces under `frontend/src/app/desktop`.
- Produces: source comments which point maintainers at the live consumer
  instead of the retired implementation.

- [x] **Step 1: Inventory production comments**

```bash
rg -n --glob '*.go' --glob '!**/*_test.go' \
  'frontend/(bindings|src/lit)|external/gui' go
```

- [x] **Step 2: Update generated-binding paths mechanically**

Replace comment-only `frontend/bindings` references with
`frontend/bindings`. Do not change identifiers, imports, JSON fields, or
runtime behaviour.

- [x] **Step 3: Map old Lit consumer paths to live Angular owners**

Use these exact route families:

```text
frontend/src/lit/views/<group>/<name>.ts
  -> frontend/src/app/desktop/surfaces/<group>/<name>.ts

frontend/src/lit/api-fetch.ts
  -> frontend/src/app/desktop/surfaces/extensions/plugin-auth-broker.ts

frontend/src/lit/app-shell.ts plugin descriptor
  -> frontend/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts

frontend/src/lit/app-shell.ts built-in ids
  -> frontend/src/app/desktop/desktop-catalogue.data.ts
     and frontend/src/app/desktop/surfaces/surface-registry.ts
```

For audit constants, point to the live TypeScript symbol discovered under
`frontend/src`; do not invent a path.

- [x] **Step 4: Verify the comment retirement**

```bash
rg -n --glob '*.go' --glob '!**/*_test.go' \
  'frontend/(bindings|src/lit)|external/gui' go
gofmt -l $(git diff --name-only --diff-filter=ACMRT -- '*.go')
go test ./go/pkg/desktop ./go/pkg/connection ./go/pkg/server ./go/pkg/audit
```

Expected: the inventory is empty, formatting prints nothing, and focused Go
packages pass.

---

### Task 3: Consolidate agent and developer documentation

**Files:**

- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `docs/development.md`

**Interfaces:**

- Consumes: authoritative manifests, Task entrypoints, and the current code
  map in `AGENTS.md`.
- Produces: one canonical agent contract plus a runnable human development
  guide.

- [x] **Step 1: Make `CLAUDE.md` defer to the canonical contract**

Retain a short repository identity, require reading `AGENTS.md`, and list only
the essential commands. Remove duplicated version snapshots, historical
coverage claims, submodule topology, and pre-Wails scaffold statements.

- [x] **Step 2: Rewrite clone, build, run, and frontend setup**

Document:

```bash
git clone <repo-url> lthn-desktop
cd lthn-desktop
go work sync
cd frontend && npm ci && cd ..
wails3 task doctor

wails3 task dev
wails3 task test
wails3 task verify:frontend
```

State that production Angular output is embedded and that Wails development
uses Angular HMR on 9245, Wails MCP on 9099, and the Lethean transport on 9199.

- [x] **Step 3: Make audit guidance truthful**

Use `bash build/audit.sh` as the repository audit entrypoint. Explain that the
external v0.9.0 compliance sweep is a diagnostic baseline with known debt,
while build/test failures remain blocking.

- [x] **Step 4: Close the stale drift section in `AGENTS.md`**

Record binding, CI, audit, active Go comments, and developer docs as retired.
Keep the tracked Lit design archive and mobile support-file cautions as
intentional reference/compatibility policy, not active drift.

- [x] **Step 5: Verify prose topology**

```bash
rg -n 'external/|frontend/src/lit|frontend/bindings|submodules|bun run' \
  AGENTS.md CLAUDE.md docs/development.md
git diff --check
```

Expected: no active-topology match; historical explanation may use the terms
only when explicitly describing what must not be restored.

---

### Task 4: Prove a normal clean checkout and finish

**Files:**

- Modify: `docs/superpowers/plans/2026-07-26-migration-retirement.md`

**Interfaces:**

- Consumes: only files committed to Git and versioned Go/npm dependencies.
- Produces: reproducible proof that local ignored snapshots and user workspace
  files are not prerequisites.

- [x] **Step 1: Run proportional checks in the working tree**

```bash
npm --prefix frontend run test:contracts
wails3 task verify:frontend
go vet ./go/...
wails3 task test
git diff --check
```

- [x] **Step 2: Commit the retirement tranche**

Stage only the plan, active Go comments, and the three documentation files.
Exclude `go.work.sum` and `.playwright-mcp/`.

```bash
git commit -m "docs(dev): finish frontend migration retirement"
```

- [x] **Step 3: Create a disposable detached worktree at the commit**

```bash
git worktree add --detach /tmp/lthn-clean-checkout-<unique> HEAD
```

The path must be newly created for this proof and removed afterwards.

- [x] **Step 4: Prove clean setup, generation, verification, and build**

Run inside the disposable worktree:

```bash
cd frontend && npm ci && cd ..
wails3 task doctor
wails3 task common:generate:bindings
wails3 task verify:frontend
npm --prefix frontend run build
```

Expected: doctor has no required errors; optional crew repositories may be
warnings. Bindings generate from `go/pkg/desktop` into
`frontend/bindings`, all frontend checks pass, and production
`go/cmd/lthn/dist/index.html` exists.

- [x] **Step 5: Run a bounded Wails development smoke**

Start `wails3 task dev`, wait for ports 9099, 9199, and 9245 plus the native
window, confirm the window loads the Angular development URL, then stop the
process. Do not leave a development app or listener running.

- [x] **Step 6: Remove only the disposable worktree**

```bash
git worktree remove /tmp/lthn-clean-checkout-<unique>
git worktree prune
```

- [x] **Step 7: Record proof, amend, and push `main`**

Add the exact clean-checkout outcome to this plan, amend the documentation
commit, rerun `git diff --check`, and push without force. Verify the remote
`main` hash and confirm the only remaining local changes are the user's
pre-existing files.

---

## Execution outcome

The working-tree retirement checks passed before the first commit:

- 35 convergence contracts passed before the macOS privacy correction;
- 268 Angular tests passed across 59 files;
- fresh aggregate coverage measured 67.28% statements and 70.97% lines;
- `go vet ./go/...`, `wails3 task test`, the focused Go packages, both Angular
  build passes, `gofmt` on changed Go files, and `git diff --check` passed; and
- the active production-Go stale-path inventory was empty.

The disposable detached worktree was created at the retirement commit and
advanced to `e5ea589` after the privacy metadata correction. From that tracked
state:

- `npm ci` installed 497 packages from the lockfile;
- `task doctor` reported every required tool, dependency, generated binding,
  optional crew checkout, and development port ready;
- desktop bindings resolved `go/pkg/desktop` into `frontend/bindings`;
- the final 36 convergence contracts and all 268 Angular tests passed;
- the production build and asset verifier passed, and
  `go/cmd/lthn/dist/index.html` existed;
- the independent second production build passed; and
- the host Wails CLI reported `v3.0.0-alpha.91`, while the compiled runtime
  correctly reported the module-pinned `v3.0.0-alpha2.117`.

The first native launch against the developer's normal home exposed a
macOS-protected-folder prerequisite: opening the existing `Documents` mount
failed closed through `io.Medium`. A focused diagnostic traced that provider
failure before any fix. Both development and production plists now carry
Documents and Downloads usage descriptions, guarded by a red/green convergence
contract; both plists pass `plutil -lint`. The Files provider still fails
closed and has no raw-path fallback.

The bounded first-run smoke then used an isolated disposable home. Angular HMR
served on 9245, Wails MCP listened on 9099, the Lethean transport listened on
9199, and Computer Use confirmed the native WebView at
`localhost:9245/#/system/telemetry`. The host created its tier-0 substrate,
loaded the desktop, and was stopped with a graceful interrupt. All three ports
were free afterwards. The disposable home was removed through a sandboxed
`io.Medium`, the disposable Git worktree was removed normally, and the
pre-existing `frontend-convergence` worktree was preserved.

Non-blocking install diagnostics remain explicit: npm reports three moderate
dependency advisories and five packages awaiting install-script review. The
native compile also emits the existing macOS deployment-version linker
warnings, and the optional `lthn-agent` sidecar is absent from `PATH` in the
isolated environment. None prevented the verified frontend or native
development paths.

---

## Plan self-review

- [x] **Scope:** already-correct binding, CI, and audit implementations are
  verified rather than rewritten.
- [x] **Retirement coverage:** active Go comments, agent guidance, developer
  setup, and clean-checkout proof cover every remaining migration-drift item.
- [x] **No placeholders:** every task names exact files, commands, and
  expected outcomes.
- [x] **Safety:** the plan preserves mobile support files, design references,
  user workspace files, and recoverable Git state.
- [x] **TDD consistency:** executable behaviour relies on existing real
  contract tests; human prose and comment-only corrections do not acquire
  brittle source-text tests.
