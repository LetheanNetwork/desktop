<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GOAL — lthn-desktop v0.9.0 compliance sweep

**Owner:** Codex (goal-mode autonomous)
**Working dir:** `/Users/snider/Code/lthn/desktop`
**Branch:** `main`
**Spawned:** 2026-05-13

---

## Objective

Drive `lthn-desktop` from **1713 findings → 0** on the canonical v0.9.0
audit. After this lands, the next goal in `core/gui/GOAL.md` becomes
viable, and once that lands too the window-position-remember feature
from `core/ide` can be ported (gated on this + core/gui both passing).

The audit is **the spec.** Every dimension explains its rule + the
canonical fix in its own preamble inside the script.

---

## Success condition

```bash
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh . 2>&1 | tail -3
```

Expected final line: `verdict: COMPLIANT`. Exit code 0.

Until that exits 0, the work is not done.

---

## Baseline (run on 2026-05-13 before this goal started)

```
verdict: NON-COMPLIANT — 1713 findings
```

Top contributors:

| Dimension              | Count | Notes |
|------------------------|------:|-------|
| ax7-triplet-gaps       |   567 | Test<File>_<Symbol>_{Good,Bad,Ugly} per symbol |
| example-gaps           |   567 | Example<Symbol> in <file>_example_test.go |
| err-shape-funcs        |   167 | `func ... error` → `func ... core.Result` |
| tuple-result-shape     |   128 | `(*T, error)` → single `core.Result` |
| banned-imports         |    90 | fmt/errors/strings/path/os/log/json/bytes → core wrappers |
| missing-example-files  |    72 | One <file>_example_test.go per public-symbol file |
| missing-test-files     |    58 | One <file>_test.go per public-symbol file |
| test-stubs             |    21 | Test bodies ≤2 lines (dispatcher gaming) |
| i18n-standalone        |    12 | `i18n.T()` → `c.I18n().Translate(key)` |
| service-canonical-shape|     9 | Add `Register(c *core.Core)` next to `NewService` |
| result-discards        |     8 | `_ = expr` discards in production |
| service-usage-example  |     8 | `// Usage example:` marker on NewService doc |
| unreferenced-tests     |     4 | Test body must name the symbol it tests |
| testify-test-files     |     2 | Swap to `core.AssertX` / `core.RequireX` |
| (everything else)      |     0 | clean |

Re-run the audit any time to see the live count. The baseline above
is just a starting orientation — the script is authoritative.

---

## Approach

Work through dimensions in the order below. Each numbered phase MUST
end with a build (`go build ./go/cmd/lthn`), a vet (`go vet ./go/...`),
a commit, and a re-run of the audit. The audit count must drop after
every phase or the phase is rolled back.

### Phase 1 — Easy mechanical sweeps

1.1. **testify-test-files (2)** — replace `import "github.com/stretchr/testify/..."`
     with `core "dappco.re/go"` and swap `assert.Equal(t, a, b)` →
     `core.AssertEqual(t, a, b)`, etc. Pattern reference: any existing
     `_test.go` in this repo that uses `core.AssertX`.

1.2. **result-discards (8)** — find `_ = expr(...)` lines in production
     (`grep -rn '^[[:space:]]*_ = .*(' --include="*.go" go/`); replace
     with `if r := expr(...); !r.OK { /* handle */ }` or propagate
     `return r`.

1.3. **i18n-standalone (12)** — find `i18n.T(`, `i18n.Label(`,
     `i18n.RegisterLocales(`, `i18n.Title(` in production. Swap to
     `c.I18n().Translate(...)` etc. via the Core service handle.

1.4. **service-usage-example (8)** — every file declaring
     `^func NewService` must contain a `// Usage example:` line in
     its file-level doc. Reference: any service file in
     `external/process/go/service.go` for the shape.

1.5. **service-canonical-shape (9)** — packages with `NewService` must
     also declare `Register(c *core.Core) core.Result` (or `RegisterCore`
     when `Register` collides with an existing init-time fn). The
     register fn typically wraps `NewService(opts)` into a `WithName`-
     compatible factory.

1.6. **test-stubs (21)** — find Test functions with bodies ≤2 lines of
     real code (helper-dispatcher gaming). Either inline the helper's
     work into the test body, or merge the helper into the test file
     so the test exercises the symbol directly.

1.7. **unreferenced-tests (4)** — the Test body must mention its target
     symbol by name (not via reflection). Find them via
     `python3 /Users/snider/Code/core/go/tests/cli/v090-upgrade/unreferenced-symbols.py .`
     and edit the test body to invoke the symbol directly.

**Phase 1 done criteria:** audit count drops below 1700 + every Phase 1
dimension reads 0. Commit message: `fix(audit): phase 1 — mechanical
sweeps`.

### Phase 2 — Banned-import migration

2.1. **banned-imports (90)** — every direct stdlib import of
     `fmt`/`errors`/`strings`/`path`/`os`/`log`/`json`/`bytes` must be
     swapped to the core wrapper. Reference table:

     | Stdlib                  | Core wrapper                                   |
     |-------------------------|------------------------------------------------|
     | `fmt.Errorf("…", err)`  | `core.E("scope", "msg", err)`                  |
     | `fmt.Sprintf`           | `core.Sprintf` (or `core.Concat` for joins)    |
     | `fmt.Println`/`Printf`  | `core.Print(core.Stderr(), …)` /  `core.Println`|
     | `errors.New`            | `core.NewError`                                |
     | `errors.Is/As/Unwrap`   | `core.Is/As/Unwrap`                            |
     | `strings.Contains` etc. | `core.Contains` etc.                           |
     | `strings.Split`         | `core.Split` / `core.SplitN`                   |
     | `strings.TrimSpace`     | `core.Trim`                                    |
     | `path.Join`             | `core.PathJoin`                                |
     | `path/filepath.Walk`    | `core.PathWalkDir`                             |
     | `os.ReadFile`           | `core.ReadFile`                                |
     | `os.WriteFile`          | `core.WriteFile`                               |
     | `os.MkdirAll`           | `core.MkdirAll`                                |
     | `os.RemoveAll`          | `core.RemoveAll`                               |
     | `os.Stat`               | `core.Stat`                                    |
     | `log.Print*`            | `core.Warn`/`core.Error`/`core.Println`        |
     | `encoding/json.Marshal` | `core.JSONMarshal`                             |
     | `encoding/json.Unmarshal`| `core.JSONUnmarshal` / `core.JSONUnmarshalString` |
     | `bytes.NewReader`       | `core.NewReader` (string variant)              |

     If a stdlib call has no core wrapper, leave that one site, file
     it as `// AX-6 EXEMPT: <reason>` and move on — do NOT invent a
     shim package or local helper to dodge the audit (that's stdlib-
     shadow-packages / local-error-helpers territory, hard-banned).

     Exempt cases observed in other repos:
     - `net/http` boundary handlers (HTTP is structural, no wrapper)
     - `os/signal` for SIGTERM trap setup
     - `unicode/utf8` for raw rune work

**Phase 2 done criteria:** banned-imports reads 0. Commit message:
`fix(audit): phase 2 — banned imports → core wrappers`.

### Phase 3 — Error → Result shape

3.1. **err-shape-funcs (167)** — convert `func X(...) error { ... }`
     to `func X(...) core.Result { ... }`. Inside the function body:
     - `return nil` → `return core.Ok(nil)` (or `core.Ok(value)`)
     - `return someErr` → `return core.Fail(core.E("scope", "msg", someErr))`
     - `if err != nil { return err }` patterns at call sites collapse
       to `if r := callee(...); !r.OK { return r }`

3.2. **tuple-result-shape (128)** — convert `func X(...) (*T, error)`
     to single-return `func X(...) core.Result`. Caller pattern:
     - `obj, err := X(...)` → `r := X(...); if !r.OK { return r }; obj := r.Value.(*T)`

3.3. **At every call site**, propagate the shape change. Tests + Examples
     for these functions need the same refactor.

**Phase 3 done criteria:** err-shape-funcs + tuple-result-shape both
read 0. Commit message: `fix(audit): phase 3 — Result-shape migration`.

### Phase 4 — Scaffolding files

4.1. **missing-test-files (58)** — find via
     `python3 /Users/snider/Code/core/go/tests/cli/v090-upgrade/file-presence.py .`
     (first line). For each source file with public symbols, create
     `<file>_test.go` next to it. Bodies can be `// TODO #1336` stubs
     for now; the symbol-level triplet check (Phase 5) populates them.
     But each created file MUST declare `package <name>` and have at
     least the boilerplate `func TestPlaceholder(t *core.T) { t.Skip("Mantis #1336") }`
     so it compiles.

4.2. **missing-example-files (72)** — same for `<file>_example_test.go`.
     Phase 5 fills the bodies; this phase just lands the file presence.

**Phase 4 done criteria:** missing-test-files + missing-example-files
both read 0. Commit message: `fix(audit): phase 4 — scaffold test +
example files`.

### Phase 5 — Triplets + Examples (BIG)

5.1. **ax7-triplet-gaps (567)** — for every public symbol, write three
     real tests in the appropriate `<file>_test.go`:

     ```go
     func Test<File>_<Symbol>_Good(t *core.T) { /* happy path */ }
     func Test<File>_<Symbol>_Bad(t *core.T)  { /* error path */ }
     func Test<File>_<Symbol>_Ugly(t *core.T) { /* edge case */ }
     ```

     The test body MUST invoke the symbol with real arguments — no
     reflect.Call() dispatch, no shared helper that takes the symbol
     name as a string. Each variant tests a genuinely different case.
     Identical bodies across Good/Bad/Ugly are flagged.

5.2. **example-gaps (567)** — for every public symbol, write at least
     one runnable Example in `<file>_example_test.go`:

     ```go
     func ExampleSymbol() {
         result := pkg.Symbol(args)
         // Output: <expected stdout if Output: present>
     }
     ```

     Example functions must compile + (if they declare `// Output:`)
     produce the expected stdout. They function as usage documentation
     under godoc.

5.3. **Cadence rule** — work one package at a time. After each package's
     triplets + examples are in, run `go test ./go/pkg/<pkg>/...` to
     verify it compiles + passes. Commit per package with message
     `test(<pkg>): AX-7 triplets + examples for #1336`.

**Phase 5 done criteria:** ax7-triplet-gaps + example-gaps both read 0.
This is the biggest phase by far — pace yourself; the audit drops by
hundreds at a time as packages get covered.

### Phase 6 — Final pass

6.1. Re-run the full audit. If any dimension is still non-zero, repeat
     the relevant phase.

6.2. Run the full Go build:
     ```bash
     cd /Users/snider/Code/lthn/desktop
     go build ./go/cmd/lthn
     go vet ./go/...
     ```
     Both must succeed.

6.3. Run the Wails bindings regeneration:
     ```bash
     cd /Users/snider/Code/lthn/desktop/go
     wails3 generate bindings -ts -d ../frontend/bindings -clean=true ./pkg/desktop/...
     ```
     Must complete without errors.

6.4. Run the frontend build to make sure nothing on the TS side broke:
     ```bash
     cd /Users/snider/Code/lthn/desktop/frontend
     bun run build
     ```

6.5. Commit final cleanup as `fix(audit): COMPLIANT — 0 findings` and
     push to `github main`.

---

## Constraints — DO NOT touch

- **`external/*` submodules** — pinned via `.gitmodules`; never edit.
- **`frontend/bindings/`** — gitignored; regenerated by wails3.
- **`build/darwin/Assets.car`** — binary asset, leave alone.
- **`docs/`** — already canon; do not rewrite for audit purposes.
- **Plugin host (`pkg/plugin/`)** — just landed; do not refactor for
  audit reasons unless a dimension specifically flags a file in it.
- **No new packages, no new repos.** Every fix lands inside this tree.
- **No `replace` directives** in `go.mod`. Workspace mode handles
  resolution. If a dep version conflict surfaces, surface it in a
  STATUS file and stop — that's a Snider-class decision.

## Stop conditions — leave a STATUS file and exit

Codex MUST stop and write `/Users/snider/Code/lthn/desktop/GOAL-STATUS.md`
with a one-page summary of what happened if any of these fire:

- Audit count goes UP between two consecutive runs (regression detected).
- A phase introduces a build break that the same phase can't fix.
- A test the codex wrote fails for a reason that requires Snider
  judgement (e.g. "this symbol shouldn't be public, should it be
  unexported?").
- 6 hours elapsed total runtime regardless of progress.

The STATUS file should include:
- Last green commit SHA
- Audit counts at start, current, last green
- The blocker description
- A recommended Snider-class next move

---

## References

- **Audit script:** `/Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh`
- **Audit helper scripts:** same directory (`ax7-gaps.py`, `file-presence.py`,
  `unreferenced-symbols.py`, `example-gaps.py`, `test-stubs.py`,
  `identical-triplets.py`)
- **AX-7 reference (the spec):** `/Users/snider/Code/host-uk/core/plans/rfc/core/RFC-CORE-008-AGENT-EXPERIENCE.md`
- **Core primitives reference:** `/Users/snider/Code/lthn/desktop/external/go/` (the
  v0.9.0 source itself — names every available wrapper)
- **Reference clean repos:** `core/agent`, `core/lint`, `core/api`
  (audit-passing v0.9.0-compliant consumers; canonical example shapes)

---

## Next goal (post-completion)

When this goal hits 0, two follow-ups unlock:

1. **`core/gui/GOAL.md`** — upgrade `dappco.re/go/gui` from
   `wails/v3 alpha.90` to `alpha.91` (which matches what `lthn-desktop`
   already consumes). See that file for the contract.

2. **Window-position-remember port** — once both goals pass, the
   `core/ide` pattern at `go/pkg/server/gui.go` lines 243-269 (calling
   `window.restore_layout` + `window.save_layout` via `core/gui`'s
   `window.Service`) can be ported into `lthn-desktop` so the per-
   surface Wails windows we just landed (editor, git, build, lint,
   containers, repos, php, marketplace, plugin) remember their
   on-screen positions across restarts.
