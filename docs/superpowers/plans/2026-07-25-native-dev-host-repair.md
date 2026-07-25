# Lethean Desktop Native Development Host Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `wails3 task dev` run the native Lethean Desktop against the
Angular HMR server, with Wails MCP on port 9099 and the Lethean WebSocket
transport on port 9199, while preserving the embedded production application
and the existing strict CSP.

**Architecture:** The existing Gin/CoreGO server remains the first handler for
registered API routes. Its no-route fallback delegates frontend requests to
Wails' `application.AssetFileServerFS`: in development Wails proxies those
requests to `FRONTEND_DEVSERVER_URL`, and without that environment variable it
serves the embedded Angular filesystem. Angular production builds retain
minification but emit a normal active stylesheet link so the strict CSP never
depends on an inline `onload` handler.

**Tech Stack:** Go 1.26, CoreGO `core.Result`/`core.FS`/`core.Handler`, Wails 3
alpha, Gin, Angular 22, npm, Node's built-in test runner, `httptest`, and the
macOS native WebView.

## Global Constraints

- Work only in the `lthn/desktop` repository. Do not modify the versioned
  `dappco.re/go*` modules, the Go module cache, or
  `/Users/snider/Downloads/Lethean-Desgin-Pack/`.
- Keep `frontend-ng/` as the sole product frontend and keep its client-side
  hash router. Do not add SSR, prerendering, hydration, or a second frontend.
- Reserve native-development listeners exactly as follows:
  Wails MCP `127.0.0.1:9099`, Lethean WebSocket transport
  `127.0.0.1:9199`, and Angular HMR `127.0.0.1:9245`.
- Do not change `pkg/connection`'s normal default of port 9099. The 9199
  override belongs only to the primary process launched by
  `build/config.yml`.
- Never put a connection token in `/wails/transport.js`, a URL, output, or
  test fixture.
- Keep Gin/CoreGO registered routes ahead of the SPA fallback. Backend routes
  must never be proxied to Angular.
- Keep production Angular output in `go/cmd/lthn/dist/`, embedded by
  `go/cmd/lthn/embed.go`.
- Keep the CSP strict. Do not add `'unsafe-inline'` to `script-src`, introduce
  a blanket nonce bypass, or rewrite generated HTML after the Angular build.
- Use British English in code, comments, tests, and documentation.
- Follow red-green-refactor for every behavioural change and commit each task
  only after its focused checks pass.
- Before native acceptance, close any previous Lethean development process and
  confirm ports 9099, 9199, and 9245 are free. Stop every process started by
  the acceptance task before handing back.

---

## Planned File Structure

New focused files:

- `go/pkg/desktop/frontend_assets.go` — the local frontend handler boundary
  that selects Wails' development proxy or embedded filesystem behaviour.
- `go/pkg/desktop/frontend_assets_internal_test.go` — shared real-handler
  tests for embedded fallback and Gin API precedence.
- `go/pkg/desktop/frontend_assets_development_internal_test.go` —
  development-tag handler tests for Angular proxying, invalid development
  URLs, and Angular routes.
- `go/pkg/desktop/frontend_assets_production_internal_test.go` —
  production-tag proof that an inherited development URL is ignored.
- `go/cmd/lthn/native_build_contract_test.go` — packaged Darwin build-tag
  contract.

Existing files changed in place:

- `build/config.yml` — supplies port 9199 only to the primary native
  development process through a cross-platform launcher.
- `scripts/wails-dev-command.mjs` — launches the build or primary Wails task
  with a deduplicated, exact development environment.
- `scripts/wails-dev-command.test.mjs` — proves executable, arguments, status,
  errors, and effective child environments with hostile ambient values through
  a platform-neutral injected spawn boundary.
- `go/cmd/lthn/mcp_wiring_test.go` — parses the ordered Wails config to pin
  command multiplicity, execution type, and build-before-run order.
- `scripts/verify-frontend-build.test.mjs` — pins CSP-compatible stylesheet
  activation; its transport fallback test remains unchanged.
- `scripts/verify-frontend-build.mjs` — verifies the generated index activates
  its stylesheet without inline JavaScript, in addition to font assets.
- `frontend-ng/angular.json` — disables production critical-CSS inlining while
  retaining production optimisation and minification.
- `go/pkg/desktop/desktop.go` — installs the Wails frontend handler as the
  existing Gin no-route fallback and corrects stale Vite wording.
- `build/darwin/Taskfile.yml` — compiles packaged macOS applications with
  Wails' `production` asset mode while preserving optional extra tags.

### Task 1: Separate the Native Development Listeners

**Files:**
- Modify: `build/config.yml`
- Modify: `go/cmd/lthn/mcp_wiring_test.go`
- Modify: `scripts/verify-frontend-build.test.mjs`
- Create: `scripts/wails-dev-command.mjs`
- Create: `scripts/wails-dev-command.test.mjs`

**Interfaces:**
- `build/config.yml` invokes `node scripts/wails-dev-command.mjs build` as one
  `blocking` entry, followed by
  `node scripts/wails-dev-command.mjs run` as one `primary` entry.
- `developmentCommandEnvironment(command, ambient)` removes every
  case-variant of `EXTRA_TAGS`, `LTHN_DEV`, `LTHN_WAILS_WS_LISTEN`, and
  `LTHN_WAILS_WS_URL` before installing the command's exact values.
- The build launcher invokes `wails3 task build`, receives
  `EXTRA_TAGS=mcp`, and receives none of the three Lethean run variables.
- The run launcher invokes `wails3 task run`, receives no `EXTRA_TAGS`, and
  receives:
  `LTHN_DEV=1`,
  `LTHN_WAILS_WS_LISTEN=127.0.0.1:9199`, and
  `LTHN_WAILS_WS_URL=ws://localhost:9199/wails/ws`.
- The standalone browser fallback in
  `frontend-ng/public/wails/transport.js` remains
  `ws://localhost:9099/wails/ws`.

- [ ] **Step 1: Write failing ordered-config and launcher tests**

In `go/cmd/lthn/mcp_wiring_test.go`, parse `build/config.yml` through the
repository's existing `gopkg.in/yaml.v3` dependency. Use:

```go
type wailsDevelopmentConfig struct {
	DevMode struct {
		Executes []struct {
			Command string `yaml:"cmd"`
			Type    string `yaml:"type"`
		} `yaml:"executes"`
	} `yaml:"dev_mode"`
}
```

Iterate the `executes` slice without folding it into a map. Count and record
the indices of:

```text
node scripts/wails-dev-command.mjs build
node scripts/wails-dev-command.mjs run
```

Assert each occurs exactly once, the build entry is `blocking`, the run entry
is `primary`, and the build index is lower than the run index.

Create `scripts/wails-dev-command.test.mjs`. Import the missing launcher
exports and write tests that:

- pass hostile values for all four managed keys, including lower-case key
  variants, and assert the returned build/run environments contain only the
  exact command-specific managed values;
- call `runDevelopmentCommand` with an injected spawn function and hostile
  ambient object, then assert the captured executable is `wails3`, arguments
  are `task build`/`task run`, `stdio` is `inherit`, and the effective managed
  environment is exact;
- avoid a fake `.cmd` executable: the real Windows Wails CLI is a native
  `wails3.exe`, and Node cannot launch `.cmd` files without a shell;
- reject an unknown launcher command.

Remove the former POSIX fake-executable command test from
`scripts/verify-frontend-build.test.mjs` and keep its transport/font tests.

- [ ] **Step 2: Run the test and confirm the port-contract failure**

Run:

```bash
go test ./go/cmd/lthn -run 'TestWailsMCPDevWiring_Good_CommandContracts' -count=1
node --test scripts/wails-dev-command.test.mjs
```

Expected: the Go test FAILS because the ordered launcher commands do not exist,
and the Node test FAILS because the launcher module does not exist.

- [ ] **Step 3: Supply the development-only transport override**

Create `scripts/wails-dev-command.mjs`. Define the managed keys and immutable
command definitions:

```js
const managedKeys = [
  'EXTRA_TAGS',
  'LTHN_DEV',
  'LTHN_WAILS_WS_LISTEN',
  'LTHN_WAILS_WS_URL',
];

const commands = Object.freeze({
  build: {
    task: 'build',
    env: { EXTRA_TAGS: 'mcp' },
  },
  run: {
    task: 'run',
    env: {
      LTHN_DEV: '1',
      LTHN_WAILS_WS_LISTEN: '127.0.0.1:9199',
      LTHN_WAILS_WS_URL: 'ws://localhost:9199/wails/ws',
    },
  },
});
```

Export `developmentCommandEnvironment(command, ambient = process.env)`.
Copy the ambient object while omitting keys whose lower-case form matches a
managed key, then add only the selected command's environment. This
case-insensitive removal prevents duplicate Windows environment keys.

Export
`runDevelopmentCommand(command, spawn = spawnSync, ambient = process.env)`.
Invoke:

```js
spawn('wails3', ['task', definition.task], {
  env: developmentCommandEnvironment(command, ambient),
  stdio: 'inherit',
});
```

Propagate launch errors and the child exit status. In the direct CLI path,
accept exactly `build` or `run`; print a concise error and exit non-zero for
anything else. Keep `shell` unset/false: production launches the native
`wails3` executable directly on every platform.

In `build/config.yml`, use:

```yaml
- cmd: node scripts/wails-dev-command.mjs build
  type: blocking
- cmd: node scripts/wails-dev-command.mjs run
  type: primary
```

Remove the now-unused `dev:build:native` and `dev:run:native` additions from
`build/Taskfile.yml`. Update the config comments to explain the 9099/9199
ownership and exact launcher environment. Do not use POSIX assignment
prefixes, the `env` executable, or Task's host-first environment defaults.

- [ ] **Step 4: Run the focused contract tests**

Run:

```bash
go test ./go/cmd/lthn -run 'TestWailsMCPDevWiring_Good_CommandContracts' -count=1
node --test scripts/wails-dev-command.test.mjs scripts/verify-frontend-build.test.mjs
```

Expected: the ordered Go config contract and all Node contract tests PASS.
The injected child-boundary test proves hostile ambient values cannot change
the effective build or run environment, arguments, executable, or inherited
stdio on either Unix or Windows.

- [ ] **Step 5: Inspect the exact diff**

Run:

```bash
git diff -- build/config.yml go/cmd/lthn/mcp_wiring_test.go \
  scripts/verify-frontend-build.test.mjs \
  scripts/wails-dev-command.mjs scripts/wails-dev-command.test.mjs
git diff --check
```

Expected: the build launcher owns only `EXTRA_TAGS=mcp`; the run launcher owns
only the three run values; config order/types are pinned; no Task helper,
POSIX-only launcher, host-first environment dependence, or whitespace error
remains.

- [ ] **Step 6: Commit the listener split**

```bash
git add build/config.yml go/cmd/lthn/mcp_wiring_test.go \
  scripts/verify-frontend-build.test.mjs \
  scripts/wails-dev-command.mjs scripts/wails-dev-command.test.mjs
git commit -m "fix(dev): separate MCP and Wails transport ports"
```

### Task 2: Route Native Frontend Requests Through Wails

**Files:**
- Create: `go/pkg/desktop/frontend_assets.go`
- Create: `go/pkg/desktop/frontend_assets_internal_test.go`
- Create: `go/pkg/desktop/frontend_assets_development_internal_test.go`
- Create: `go/pkg/desktop/frontend_assets_production_internal_test.go`
- Create: `go/cmd/lthn/native_build_contract_test.go`
- Modify: `go/pkg/desktop/desktop.go`
- Modify: `build/darwin/Taskfile.yml`
- Read: `go/pkg/server/service.go`
- Test: `go/pkg/server/server_test.go`

**Interfaces:**
- Produces the unexported local boundary:
  `frontendAssetHandler(assets core.FS) core.Handler`.
- `Service.attachSPA()` still returns `core.Result`.
- Explicit server routes such as `/health` remain Gin/CoreGO responses.
- Unmatched frontend requests use `application.AssetFileServerFS`.
- With `FRONTEND_DEVSERVER_URL` set, the handler uses the development server
  in a build without the `production` tag and never silently falls back to
  embedded output.
- With the `production` tag, the handler ignores
  `FRONTEND_DEVSERVER_URL` and serves the supplied embedded filesystem.
- Packaged Darwin builds always include the `production` tag, composed with
  any optional extra tags.

- [ ] **Step 1: Write the failing real-handler tests**

Create `go/pkg/desktop/frontend_assets_internal_test.go` with the repository
licence header, `package desktop`, and these imports:

```go
import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/server"
)
```

Add a small test-only constructor:

```go
func newFrontendAssetTestService(frontend core.FS) (*Service, *server.Service) {
	backend := server.NewService(server.Options{})
	desktop := NewService(Options{
		Frontend: frontend,
		Server:   backend,
	})
	return desktop, backend
}
```

Add
`TestFrontendAssets_Good_DevelopmentServerHandlesAssetsAndRoutes`. Start an
`httptest.Server` whose handler records the path and returns:

- `font/woff2` plus body `development-font` for
  `/media/probe.woff2`;
- `text/html; charset=utf-8` plus body `development-route` for
  `/system/telemetry`;
- `404` for any unexpected path.

Set `FRONTEND_DEVSERVER_URL` to that server's URL with `t.Setenv`. Construct
the desktop service over:

```go
fstest.MapFS{
	"dist/index.html": &fstest.MapFile{Data: []byte("embedded-index")},
}
```

Call `attachSPA()` and require an OK result. Send both paths through
`backend.Handler()` and assert status 200, the development response body, and
`font/woff2` for the font. Also assert the development server observed both
paths.

Add `TestFrontendAssets_Good_RegisteredBackendRouteStaysLocal`. Use the same
development server, but return body `wrong-handler` for every request. After
`attachSPA()`, request `/health` through `backend.Handler()` and assert:

```go
core.AssertEqual(t, core.StatusOK, response.Code)
core.AssertTrue(t, core.Contains(response.Body.String(), `"data":"healthy"`))
core.AssertFalse(t, core.Contains(response.Body.String(), "wrong-handler"))
```

Add `TestFrontendAssets_Ugly_EmbeddedFilesystemUsedWithoutDevelopmentURL`.
Set `FRONTEND_DEVSERVER_URL` to the empty string, construct:

```go
fstest.MapFS{
	"dist/index.html":        &fstest.MapFile{Data: []byte("embedded-index")},
	"dist/media/probe.woff2": &fstest.MapFile{Data: []byte("embedded-font")},
}
```

After `attachSPA()`, request `/media/probe.woff2` and assert status 200, body
`embedded-font`, and a response content type that is not `text/html`.

Add `TestFrontendAssetHandler_Bad_InvalidDevelopmentURLFailsExplicitly`. Set
`FRONTEND_DEVSERVER_URL` to `://invalid`, call the missing
`frontendAssetHandler` directly over a minimal `fstest.MapFS`, and assert a
request returns status 500 rather than embedded content.

- [ ] **Step 2: Run the tests and confirm the missing-boundary failure**

Run:

```bash
go test ./go/pkg/desktop -run 'TestFrontendAsset'
```

Expected: FAIL to compile because `frontendAssetHandler` is undefined. The
test must fail before `desktop.go` is changed.

- [ ] **Step 3: Implement the local Wails frontend boundary**

Create `go/pkg/desktop/frontend_assets.go`:

```go
// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// frontendAssetHandler serves the embedded Angular filesystem in compiled
// builds and proxies it to FRONTEND_DEVSERVER_URL during Wails development.
func frontendAssetHandler(assets core.FS) core.Handler {
	return application.AssetFileServerFS(assets)
}
```

Keep this helper unexported: this is a downstream prototype boundary, not a
new public framework API.

- [ ] **Step 4: Install the handler as Gin's no-route fallback**

In `go/pkg/desktop/desktop.go`, change:

```go
fileServer := core.HTTPFileServer(core.HTTPFS(sub))
```

to:

```go
fileServer := frontendAssetHandler(sub)
```

Do not change `s.opts.Server.Engine().SetNoRoute(...)` or move the frontend
handler ahead of the Gin engine.

Update nearby comments and `Options.Frontend` documentation from “embedded
Vite” to “embedded Angular”. Explain that the no-route handler proxies to
Angular in Wails development and serves embedded output otherwise. Do not
alter the `/wails/*` middleware.

- [ ] **Step 5: Run the focused Go tests**

Run:

```bash
go test ./go/pkg/desktop -run 'TestFrontendAsset'
go test ./go/pkg/server -run 'TestServer_Health_Good|TestServer_CSP'
```

Expected: PASS. The desktop test output proves:

- development font and Angular-route requests reach the test server;
- `/health` remains local;
- no development URL serves the embedded font;
- an invalid development URL returns an explicit failure.

- [ ] **Step 6: Run the complete changed-package checks**

Run:

```bash
gofmt -w go/pkg/desktop/frontend_assets.go go/pkg/desktop/frontend_assets_internal_test.go
go test ./go/pkg/desktop ./go/pkg/server
go vet ./go/pkg/desktop ./go/pkg/server
git diff --check
```

Expected: all commands PASS. If the broad package tests expose a pre-existing
environmental port collision, first confirm that no development app is
running; do not weaken the test or change production listener defaults.

- [ ] **Step 7: Commit the native asset boundary**

```bash
git add go/pkg/desktop/desktop.go \
  go/pkg/desktop/frontend_assets.go \
  go/pkg/desktop/frontend_assets_internal_test.go
git commit -m "fix(dev): proxy native SPA assets to Angular HMR"
```

- [ ] **Step 8: Add failing production-mode and Darwin build contracts**

Move
`TestFrontendAssets_Good_DevelopmentServerHandlesAssetsAndRoutes` and
`TestFrontendAssetHandler_Bad_InvalidDevelopmentURLFailsExplicitly` into
`go/pkg/desktop/frontend_assets_development_internal_test.go`, guarded by:

```go
//go:build !production
```

Keep shared constructors, the registered-backend precedence test, and the
embedded-filesystem test in the untagged file. Strengthen the embedded font
assertion to require exactly `font/woff2`.

Create
`go/pkg/desktop/frontend_assets_production_internal_test.go`, guarded by:

```go
//go:build production
```

Its test must set `FRONTEND_DEVSERVER_URL` to a live `httptest.Server`, install
an embedded `dist/media/probe.woff2`, and prove:

- the response is status 200 with body `embedded-font`;
- the content type is exactly `font/woff2`;
- the development server receives zero requests.

Create `go/cmd/lthn/native_build_contract_test.go`. Read
`../../../build/darwin/Taskfile.yml` and assert its non-development
`BUILD_FLAGS` branch contains:

```text
-tags production{{if .EXTRA_TAGS}},{{.EXTRA_TAGS}}{{end}}
```

Run:

```bash
go test -tags production ./go/pkg/desktop -run 'TestFrontendAsset' -count=1
go test ./go/cmd/lthn -run 'TestNativeProductionAssets_Good_DarwinBuildUsesProductionTag' -count=1
```

Expected before changing the Darwin task: the production handler test passes
after the test split, while the Darwin build contract FAILS because its
release branch omits `production`.

- [ ] **Step 9: Compile packaged Darwin applications in production asset mode**

In `build/darwin/Taskfile.yml`, change only the non-development
`BUILD_FLAGS` branch to match the established Linux/Windows composition:

```yaml
-tags production{{if .EXTRA_TAGS}},{{.EXTRA_TAGS}}{{end}} -trimpath -buildvcs=false
```

Keep development flags unchanged so `wails3 task dev` can proxy to Angular.
Do not set or clear `FRONTEND_DEVSERVER_URL` in the packaged process; the
compile-time Wails production handler must make it irrelevant.

- [ ] **Step 10: Verify both compiled asset modes**

Run:

```bash
gofmt -w \
  go/cmd/lthn/native_build_contract_test.go \
  go/pkg/desktop/frontend_assets_internal_test.go \
  go/pkg/desktop/frontend_assets_development_internal_test.go \
  go/pkg/desktop/frontend_assets_production_internal_test.go
go test ./go/pkg/desktop -run 'TestFrontendAsset' -count=1
go test -tags production ./go/pkg/desktop -run 'TestFrontendAsset' -count=1
go test ./go/cmd/lthn -run 'TestNativeProductionAssets_Good_DarwinBuildUsesProductionTag' -count=1
go test ./go/pkg/desktop ./go/pkg/server ./go/cmd/lthn
go vet ./go/pkg/desktop ./go/pkg/server ./go/cmd/lthn
git diff --check
```

Expected: all commands PASS. Normal compilation proves the HMR proxy and
invalid-URL behaviour; production compilation proves the inherited
development URL is ignored and embedded assets remain authoritative.

- [ ] **Step 11: Commit the production-mode correction**

```bash
git add build/darwin/Taskfile.yml \
  go/cmd/lthn/native_build_contract_test.go \
  go/pkg/desktop/frontend_assets_internal_test.go \
  go/pkg/desktop/frontend_assets_development_internal_test.go \
  go/pkg/desktop/frontend_assets_production_internal_test.go
git commit -m "fix(build): force production assets on macOS"
```

### Task 3: Make Production Styles Compatible With the Strict CSP

**Files:**
- Modify: `scripts/verify-frontend-build.test.mjs`
- Modify: `scripts/verify-frontend-build.mjs`
- Modify: `frontend-ng/angular.json`
- Read: `go/pkg/server/handlers.go`
- Read: `frontend-ng/src/foundations/_tokens.scss`

**Interfaces:**
- `verifyFrontendBuild(distDir)` continues to validate all referenced font
  assets and the four required font families.
- Its report additionally contains `stylesheetLinks: string[]`.
- The verifier rejects:
  - a build with no stylesheet link;
  - a stylesheet link containing an inline `onload`;
  - a stylesheet link restricted to `media="print"`.
- Generated-index parsing ignores HTML comments, matches exact attribute
  names, supports quoted and unquoted values, tokenises `rel`
  case-insensitively, trims `media`, and validates every stylesheet link.
- Angular's production build keeps script/style minification but sets
  `optimization.styles.inlineCritical` to `false`.

- [ ] **Step 1: Add stylesheet activation fixtures**

In `scripts/verify-frontend-build.test.mjs`, define:

```js
const activeIndex =
  '<!doctype html><html><head>' +
  '<link rel="stylesheet" href="styles.css">' +
  '</head><body></body></html>';
```

Write that `index.html` into both existing temporary build fixtures before
calling `verifyFrontendBuild`.

Add a complete-font fixture test named
`stylesheet verification rejects inline critical activation`. Give it the
same valid CSS and four real temporary font files as the successful font test,
but write a stylesheet link with `onload` that is not also print-only:

```html
<!doctype html><html><head>
  <link rel="stylesheet" href="styles.css"
        onload="this.media='all'">
</head><body></body></html>
```

Assert:

```js
await assert.rejects(
  verifyFrontendBuild(root),
  /stylesheet activation depends on inline script/,
);
```

Add a separate complete-font fixture named
`stylesheet verification rejects print-only links` whose index contains:

```html
<!doctype html><html><head>
  <link rel="stylesheet" href="styles.css" media="print">
</head><body></body></html>
```

Assert the same
`/stylesheet activation depends on inline script/` rejection. Keeping
`onload` out of this fixture proves the print-only predicate independently.

Add a third complete-font fixture named
`stylesheet verification rejects a missing stylesheet link`. Its index may
contain a non-stylesheet `<link rel="icon">`, but no stylesheet link. Assert
rejection with:

```js
/missing active stylesheet link/
```

Use a small test helper for the repeated valid CSS/font fixture if useful, but
keep every test's `index.html` explicit enough that the failed activation
contract is visible at the call site.

Extend the successful fixture to assert:

```js
assert.deepEqual(report.stylesheetLinks, [
  '<link rel="stylesheet" href="styles.css">',
]);
```

- [ ] **Step 2: Run the Node tests and confirm the false-negative failure**

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
```

Expected: FAIL because the current verifier accepts invalid or missing
stylesheet activation and does not return `stylesheetLinks`.

- [ ] **Step 3: Implement generated-index verification**

In `scripts/verify-frontend-build.mjs`, read
`join(distDir, 'index.html')`. Extract every `<link ...>` tag without assuming
attribute order:

```js
const stylesheetLinks = [...indexHTML.matchAll(/<link\b[^>]*>/gi)]
  .map((match) => match[0])
  .filter((tag) => /\brel\s*=\s*["']stylesheet["']/i.test(tag));
```

Apply these checks before returning:

```js
if (stylesheetLinks.length === 0) {
  throw new Error('missing active stylesheet link');
}
for (const link of stylesheetLinks) {
  const usesInlineActivation = /\bonload\s*=/i.test(link);
  const isPrintOnly = /\bmedia\s*=\s*["']print["']/i.test(link);
  if (usesInlineActivation || isPrintOnly) {
    throw new Error(`stylesheet activation depends on inline script: ${link}`);
  }
}
```

Return:

```js
return {
  requiredFamilies: REQUIRED_FAMILIES,
  references,
  missingAssets,
  stylesheetLinks,
};
```

Update the command-line success message to include the stylesheet-link count.
Keep the font checks and their error messages intact.

- [ ] **Step 4: Run the fixture tests**

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
```

Expected: all tests PASS. This establishes that the verifier can distinguish a
normal stylesheet from Angular's inline critical-CSS activation.

- [ ] **Step 5: Prove the current production build violates the contract**

Run:

```bash
cd frontend-ng
npm run build
npm run verify:build
```

Expected before changing `angular.json`: `npm run build` succeeds and
`npm run verify:build` FAILS with
`stylesheet activation depends on inline script`, showing the generated
print/onload link.

- [ ] **Step 6: Disable only critical-CSS inlining**

In `frontend-ng/angular.json`, change:

```json
"inlineCritical": true
```

to:

```json
"inlineCritical": false
```

Leave `scripts: true`, `minify: true`, `removeSpecialComments: true`, and
`fonts: false` unchanged. Do not modify the CSP.

- [ ] **Step 7: Rebuild and verify the production output**

Run:

```bash
cd frontend-ng
npm run build
npm run verify:build
```

Expected: PASS. Inspect the generated index:

```bash
rg -n '<link[^>]+stylesheet|onload=|media=\"print\"' ../go/cmd/lthn/dist/index.html
```

Expected: at least one normal `rel="stylesheet"` link, with no `onload=` and
no `media="print"`.

- [ ] **Step 8: Run the complete frontend contracts**

Run:

```bash
cd frontend-ng
npm run test:contracts
npm run build
npm run verify:build
cd ..
git diff --check
```

Expected: all commands PASS. The production build still declares Geist, Geist
Mono, Instrument Serif, and Font Awesome, and every referenced font file
exists.

- [ ] **Step 9: Commit the CSP-compatible build**

```bash
git add frontend-ng/angular.json \
  scripts/verify-frontend-build.mjs \
  scripts/verify-frontend-build.test.mjs
git commit -m "fix(frontend): keep production styles CSP compatible"
```

Do not stage `go/cmd/lthn/dist/`; it is generated build output.

- [ ] **Step 10: Add failing generated-index parser regressions**

Extend `scripts/verify-frontend-build.test.mjs` with complete-font fixtures
that independently prove:

- `data-rel="stylesheet"` without a true `rel` is rejected as a missing
  stylesheet;
- a stylesheet present only inside an HTML comment is ignored and rejected as
  missing;
- `data-onload="marker"` on an otherwise valid stylesheet is not treated as
  an inline event handler;
- unquoted `rel=stylesheet` is accepted;
- `media=" print "` is trimmed and rejected as print-only;
- reordered, mixed-case attributes with a tokenised value such as
  `REL="preload StyleSheet"` are accepted;
- when multiple stylesheet links exist, an invalid second link is still
  rejected.

Keep the index markup visible in each case or use a table whose case names and
markup make the boundary explicit.

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
```

Expected before changing the parser: the new cases expose the current
substring-regex false positives and false negatives.

- [ ] **Step 11: Parse actual link attributes**

In `scripts/verify-frontend-build.mjs`, strip HTML comments before extracting
`link` tags. Parse each tag's whitespace-delimited attributes into exact
case-insensitive names with values accepted in double-quoted, single-quoted,
or unquoted form.

Select stylesheets when the exact `rel` attribute contains a
case-insensitive whitespace-delimited `stylesheet` token. Detect inline
activation only when the exact `onload` attribute exists. Treat the exact
`media` value as print-only after trimming whitespace and folding case.
Validate every selected stylesheet link and continue to return each original
tag verbatim in `stylesheetLinks`.

Do not add an HTML parser dependency for this generated Angular index. Keep
the helpers local and focused, and do not weaken any font check.

- [ ] **Step 12: Correct the command-line report and rerun contracts**

Change the CLI prefix from `frontend font build` to `frontend build`. Avoid
singular/plural ambiguity by formatting the count as:

```text
stylesheet links: 1
```

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
cd frontend-ng
npm run test:contracts
npm run build
npm run verify:build
cd ..
rg -n '<link[^>]+stylesheet|onload=|media="print"' go/cmd/lthn/dist/index.html
git diff --check
```

Expected: all tests and the production build/verifier PASS; the real
generated index still contains one normal active stylesheet and no inline or
print-only activation.

- [ ] **Step 13: Commit the Task 3 parser correction**

Stage only:

```bash
git add scripts/verify-frontend-build.mjs \
  scripts/verify-frontend-build.test.mjs
git commit -m "fix(frontend): parse generated stylesheet links"
```

Do not stage generated `go/cmd/lthn/dist/`. Return the updated review range
based at `5bdab24` for the same specification and code-quality reviewers.

### Task 4: Prove Native HMR and Embedded Production End to End

**Files:**
- Verify: `build/config.yml`
- Verify: `go/pkg/desktop/frontend_assets.go`
- Verify: `go/pkg/desktop/desktop.go`
- Verify: `frontend-ng/angular.json`
- Verify: `frontend-ng/src/foundations/_tokens.scss`
- Verify: `frontend-ng/src/foundations/_icons.scss`
- Verify: `go/cmd/lthn/dist/index.html`
- Record evidence in:
  `.superpowers/sdd/2026-07-25-frontend-convergence/task-3-report.md`
  and
  `.superpowers/sdd/2026-07-25-frontend-convergence/progress.md`
  without staging those ignored SDD files.

**Acceptance contract:**
- `wails3 task dev` starts without any caller-supplied port override.
- Angular HMR listens on 9245, Wails MCP on 9099, and Lethean transport on
  9199.
- The native WebView receives Angular development assets rather than stale
  embedded assets.
- Strict CSP remains unchanged.
- Darwin uses the SF sans/mono token policy, Instrument Serif is loadable,
  and a real Font Awesome icon has non-empty generated content.
- The production build remains valid and independently usable without the
  development server.

- [ ] **Step 1: Run the pre-native focused gate**

Run from the repository root:

```bash
node --test scripts/verify-frontend-build.test.mjs
go test ./go/pkg/connection ./go/pkg/desktop ./go/pkg/server
go vet ./go/pkg/connection ./go/pkg/desktop ./go/pkg/server
cd frontend-ng
npm run test:contracts
npm run build
npm run verify:build
cd ..
git diff --check
```

Expected: every command PASS and the worktree contains only intended tracked
changes or is clean after the three task commits.

- [ ] **Step 2: Establish a clean listener baseline**

Confirm no Lethean or Angular development process is running:

```bash
lsof -nP -iTCP:9099 -iTCP:9199 -iTCP:9245 -sTCP:LISTEN
pgrep -af 'lthn|wails3 dev|ng serve'
```

Expected: no listener on 9099, 9199, or 9245. If a process belongs to this
worktree's prior smoke run, stop it cleanly before proceeding. Do not kill
unrelated user processes.

- [ ] **Step 3: Start the supported native development flow**

Read and use the `computer-use:computer-use` skill before controlling the
native app. Start:

```bash
wails3 task dev
```

Keep this one command in flight. Wait for the Angular development server,
native binary, and app window instead of starting a second development
session.

Expected logs include a frontend server on 9245 and a successful native
launch. There must be no `address already in use` error.

- [ ] **Step 4: Prove listener and generated-transport ownership**

While the app is open, run:

```bash
lsof -nP -iTCP:9099 -iTCP:9199 -iTCP:9245 -sTCP:LISTEN
```

Expected:

- the Wails development process owns 9099;
- the Lethean application owns 9199;
- Angular owns 9245.

From the native WebView, fetch `/wails/transport.js` and evaluate the frozen
`globalThis.__LTHN_CONNECTION__`. Require:

```json
{"webSocketUrl":"ws://localhost:9199/wails/ws"}
```

Assert the response and object expose no `token`, `accessToken`,
`authorization`, or bearer value.

- [ ] **Step 5: Prove native requests use Angular HMR**

Use the Wails development evaluator or native WebView inspection to fetch one
actual font URL referenced by the development stylesheet. Assert status 200
and a non-HTML font MIME type.

Then touch an Angular source file without changing its contents:

```bash
touch frontend-ng/src/styles.scss
```

Expected: the existing `ng serve` process reports a rebuild/HMR update and the
already-open native window remains connected. Confirm no Go rebuild and no
embedded-production rebuild was required. `git status --short` must remain
unchanged by the timestamp-only touch.

- [ ] **Step 6: Prove platform typography and icon rendering**

In the native WebView, evaluate the live DOM and computed styles:

- `document.documentElement.dataset.platform` equals `darwin`;
- `getComputedStyle(document.documentElement).getPropertyValue('--font-sans')`
  contains the SF sans policy;
- `getComputedStyle(document.documentElement).getPropertyValue('--font-mono')`
  contains the SF mono policy;
- `document.fonts.load('16px "Instrument Serif"')` resolves with at least one
  loaded face;
- a visible Font Awesome icon's computed `font-family` contains
  `Font Awesome 7 Free`;
- that icon's computed `font-weight` matches the declared icon style;
- `getComputedStyle(icon, '::before').content` is neither empty nor `none`.

Capture one native screenshot showing the populated icons. Record the
evaluated values and the screenshot path in the ignored Task 3 SDD report.

- [ ] **Step 7: Stop development cleanly**

Stop `wails3 task dev` through its controlling terminal. Wait for the native
window, Angular child process, and Wails development process to exit. Re-run:

```bash
lsof -nP -iTCP:9099 -iTCP:9199 -iTCP:9245 -sTCP:LISTEN
pgrep -af 'lthn|wails3 dev|ng serve'
```

Expected: no process from this acceptance run remains. Investigate and stop
only descendants of the development command if cleanup is incomplete.

- [ ] **Step 8: Re-prove embedded production with no development server**

Run:

```bash
cd frontend-ng
npm run build
npm run verify:build
cd ..
go test ./go/pkg/desktop ./go/pkg/server ./go/pkg/connection
git status --short --branch
```

Expected: production build and focused Go tests PASS with ports 9099, 9199,
and 9245 unused. The generated index has an active stylesheet and the
production font verifier passes without `FRONTEND_DEVSERVER_URL`.

- [ ] **Step 9: Hand the repaired prerequisite back to the parent plan**

Append exact command results, listener owners, WebView values, HMR evidence,
and cleanup evidence to:

```text
.superpowers/sdd/2026-07-25-frontend-convergence/task-3-report.md
```

Update the ignored SDD ledger entry for Task 3 from blocked to ready for
independent review. Do not create a no-op commit for smoke evidence and do not
stage `.superpowers/sdd/`.

The parent `2026-07-25-frontend-convergence.md` workflow must now:

1. run Task 3's independent task review over the complete Task 3 commit range;
2. resolve any review findings;
3. mark Task 3 complete only after that review passes;
4. continue with Task 4 of the parent convergence plan.

---

## Final Verification Matrix

| Contract | Evidence |
|---|---|
| Wails MCP owns 9099 | ordered config plus hostile-ambient launcher test and live `lsof` |
| Lethean native transport owns 9199 | ordered config, hostile-ambient launcher test, live `lsof`, generated transport object |
| Browser/default transport remains 9099 | existing fallback transport contract test |
| Native assets reach Angular HMR | Go proxy test, native font fetch, HMR rebuild |
| Backend routes remain local | `/health` real-handler test |
| Compiled app serves embedded assets | no-dev Go handler test and post-smoke production build |
| CSP stays strict | no CSP source change plus generated-index verifier |
| Full stylesheet is active | production build has normal stylesheet link, no print/onload activation |
| Darwin typography applies | native computed token values |
| Instrument Serif remains available | native `document.fonts.load` and build font verifier |
| Font Awesome renders | native computed family/weight/pseudo-content and screenshot |
| Development shuts down cleanly | empty listener and process checks after stop |

## Completion Rule

This focused plan is complete only when all four tasks are green, the native
acceptance uses plain `wails3 task dev` with no smoke-only override, all
started processes have stopped, and the parent frontend-convergence Task 3 has
enough evidence to enter independent review. Passing unit tests without the
native HMR smoke is not completion.
