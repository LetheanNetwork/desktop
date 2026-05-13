<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Agent Notes

This repository is the Lethean Desktop product binary — `lthn`. It is a CLI router that dispatches to subsystems (GUI, HTTP server, AI runtime, future blockchain / LNS / wallet modules). The Wails GUI is one consumer of that dispatch, not the binary's identity.

## Repository layout

Canonical Lethean Go repo shape:

- `go/` — the Go module (`dappco.re/lthn/desktop`). All Go code lives here.
- `external/` — git submodules of canonical Lethean dependencies, pinned to `dev` branches via `.gitmodules`.
- `go.work` at repo root — workspace mode points at `./go` + `./external/*`. Live dev sources resolve through here.
- `frontend/`, `docs/`, `bin/`, `build/`, `LICENCE`, `README.md`, `CLAUDE.md`, `AGENTS.md`, `Taskfile.yml` at repo root.

## Code Map

- `go/cmd/lthn/main.go` — CLI router. Parses `core.Args()`, dispatches on subcommand (`version`, `help`, `gui`, `tray`, `serve`, `ai`). Add new subcommands here as flat handlers that delegate to `go/pkg/*`.
- `go/pkg/tray/tray.go` — NSStatusItem + popover anchor + window-spawn router (consumed by `lthn gui`).
- `go/pkg/runner/service.go` — go-mlx adapter signals contract (consumed by `lthn ai` and `lthn serve`).
- `go/pkg/telemetry/service.go` — `powermetrics` / `IOReport` sampler.
- `external/go/` — submodule of `dappco.re/go` (the Core primitives module) on its `dev` branch.
- `frontend/` — Vite + Lit. Lethean-5 components in `src/lit/`. `index.html` is the app entry; `canvas.html` is the design canvas.
- `docs/design/lethean-4-react-reference/` — animated React/JSX visual source for design review only; not built.

Each package in `pkg/` follows the Mantis #1336 canonical Service.go shape: a `Service` struct, a `NewService(opts Options) *Service` constructor, a `(s *Service) Register(c *core.Core) core.Result` method, AND a free `Register(c *core.Core) core.Result` function for one-shot wiring. Files declaring `NewService` carry a `// Usage example:` doc marker per Mantis #1383.

## Compliance Rules

Follow the v0.9.0 Core compliance shape. Use `dappco.re/go` wrappers for output (`core.Print`, `core.Println`, `core.Sprintf`), argv (`core.Args()`), exit (`core.Exit`), flag parsing (`core.ParseFlag`), errors (`core.E`, `core.NewError`), and results (`core.Result`, `core.Ok`, `core.Fail`). Direct stdlib imports of `fmt`, `errors`, `strings`, `os`, `log`, `encoding/json`, `bytes`, `path`, `path/filepath`, `os/exec`, `io/ioutil` are banned in production AND test files.

Function signatures return `core.Result`, never `error` and never `(T, error)` tuples. The Result type recovers panics inside the function body, so callers branch on `r.OK` and pull the value from `r.Value`.

Do not roll your own primitives that CoreGO already provides. Check `dappco.re/go/*.go` for the canonical wrapper before importing stdlib.

Use TDD when adding code. Each new public symbol ships with `Test<File>_<Symbol>_{Good,Bad,Ugly}` triplets in the matching `<file>_test.go` and at least one `Example<Symbol>` in `<file>_example_test.go`. The pre-existing scaffold has a test-scaffolding backlog flagged by the v0.9.0 audit (`ax7-triplet-gaps`, `example-gaps`, `missing-test-files`, `missing-example-files`) — file the gap as it gets filled, do not extend the backlog with new untested public symbols.

## Testing

The repo has a paired Go + frontend test foundation. Coverage target is ≥70% per package — proves the code works and catches regressions. Pushing individual packages to 80%+ is an open agent workstream.

### One-shot entrypoints (Taskfile)

```bash
wails3 task test                 # run both suites
wails3 task test:go              # Go pkg/... only
wails3 task test:frontend        # Vitest only

wails3 task test:cover           # both with coverage reports
wails3 task test:cover:go        # → go/coverage.{out,html} + func table
wails3 task test:cover:frontend  # → frontend/coverage/index.html
```

`test:cover` prints the report paths + parse recipes (`go tool cover -func=coverage.out` for the table; `open` for the HTML viewer).

### Go test canon — `core/go` framework

Every test file uses the AX-shaped pattern from `dappco.re/go`:

```go
package mypkg_test

import core "dappco.re/go"
import "dappco.re/lthn/desktop/pkg/mypkg"

func TestMyFunc_Good(t *core.T) {
    r := mypkg.MyFunc("ok")
    core.AssertTrue(t, r.OK)
    core.AssertEqual(t, "expected", r.Value.(string))
}

func TestMyFunc_Bad_EmptyInput(t *core.T) {
    r := mypkg.MyFunc("")
    core.AssertFalse(t, r.OK)
}
```

Rules:
- External `_test` package (no internal-symbol leak — except for table-driving unexported helpers, which goes in a `package mypkg` internal_test)
- Aliased `dappco.re/go` import gives `core.T`, `core.AssertEqual`, `core.AssertTrue`, …. **No separate `import "testing"` line.**
- AX naming convention: `TestFunc_Good`, `TestFunc_Bad_<reason>`, `TestFunc_Ugly_<reason>`.
- HOME-isolated fixtures (`t.TempDir()` + `os.Setenv("HOME", tmp)` + `t.Cleanup`) for anything that touches `~/Lethean/`.
- `core.AssertError`'s variadic strings are **substring requirements**, not failure messages — pass none when you only care that err is non-nil.

### Frontend test canon — Vitest + happy-dom

Stack: vitest@^3, happy-dom, @vitest/coverage-v8@^3. Vite config carries the `test` block so path aliases (`@service`, `@ui`, …) work in tests too.

Shared fixture at `frontend/src/test/window-fixture.ts`:

```ts
import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./my-window";

describe("lthn-my-window", () => {
  it("mounts with the right title", async () => {
    const { host } = await mountWindow("lthn-my-window");
    expectChromeTitle(host, "My Window");
  });
});
```

Canonical 4-section pattern per window:
1. **Smoke** — mounts without throwing + titlebar carries the right title.
2. **Embedded sweep** — `embedded` attribute collapses chrome to `.lthn-window--embedded`.
3. **Content presence** — distinctive strings from the body so render regressions fail loudly.
4. **Reactive prop** (where relevant) — mutate, `await el.updateComplete`, assert the rendered DOM reflected.

### Coverage outliers — accepted, not bugs

- **`pkg/services` (49.4%)** — kardianos/service writes to `~/Library/LaunchAgents/` and `~/.config/systemd/user/`. Unit tests must not mutate those. Integration-suite responsibility (container or disposable VM).
- **`pkg/desktop` (9.3%)** — most of the 1882 LOC is inside `Service.Run()` which boots Wails. Headless-Wails integration is its own workstream.

Both are documented in their test files. Don't fight the ceilings; lift the other packages.

### Open agent workstream — `>70% → 80%+`

Run `wails3 task test:cover:go`, open `go/coverage.html`, pick a package, add targeted tests for uncovered branches, re-run, commit.

| Package | Current | Headroom |
|---|---|---|
| `pkg/runner` | 72.7% | router-backed Generate / Chat path (needs an httptest openai-mock backend) |
| `pkg/sessions` | 75.6% | store-error propagation arms |
| `pkg/permissions` | 78.3% | EntitlementChecker closure with a config service registered |
| `pkg/telemetry` | 85.7% | NewService nil-Core sweep |
| `pkg/firstlaunch` | 87.0% | yamlHasRoutes edge cases |

## Before Stopping

Workspace mode is the bar. From the repo root with `go.work` active:

```bash
go work sync
go vet ./go/...
wails3 task test                              # both suites
gofmt -l go/
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh .
```

The audit reports compliance dimensions. Eight code-wrongness dimensions (`banned-imports`, `err-shape-funcs`, `tuple-result-shape`, `result-discards`, `service-canonical-shape`, `service-usage-example`, `service-name-empty`, `legacy-imports`) should stay at zero on every commit. Test/example/docs completeness dimensions may carry a backlog while scaffold work continues; do not regress what is already at zero.

Frontend builds with Vite. From `frontend/`:

```bash
npm install
npm run build       # → frontend/dist/
npm run dev         # → http://127.0.0.1:9245/
npm test            # vitest run
```

The Wails GUI consumer of the binary is decoupled — `lthn serve` and `lthn ai` must function with the GUI broken. Test against this property when changing dispatch.
