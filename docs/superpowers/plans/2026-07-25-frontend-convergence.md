# Lethean Desktop Frontend Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `frontend-ng/` the sole product frontend, preserve the working native-WebView and separable WebSocket transport contracts, repair migration drift, verify the design-pack typography path, and retire duplicate frontend archives without losing recoverable work.

**Architecture:** Angular 22 remains one client-side-rendered application with one hash router and one NgRx window state, presented as Desktop OS, App Shell, phone, tablet, tray, or standalone native window. Wails remains the native host (WebView2 on Windows and the platform WebView elsewhere), while `pkg/connection` and `ConnectionManagerService` keep generated binding calls transport-independent so a separately served GUI can use the same backend later. The portable Lit elements and Sass token implementation stay inside `frontend-ng/`; only superseded whole-application copies are retired.

**Tech Stack:** Go 1.26, the `dappco.re/go*` CoreGO framework, Wails 3 alpha, Angular 22 standalone components, TypeScript 6, NgRx 21, Lit 3, Vitest/jsdom, npm, Sass, self-hosted `@fontsource` assets, Font Awesome 7.

## Global Constraints

- `frontend-ng/` is the canonical and only product frontend.
- Keep Angular client-side rendered and hash-routed; do not add SSR, prerendering as an application mode, or hydration.
- Preserve `#/`, `#/w/:app`, and `#/tray`, plus all existing category/app/child route IDs.
- Angular Router remains the navigation source of truth; NgRx remains the durable window-state source.
- Desktop, App Shell, phone, and tablet remain presentations of the same route catalogue and component instances.
- Keep `frontend-ng/src/foundations/`, `frontend-ng/src/kit/`, the `lit` dependency, and `kind: "lit"` plugin support.
- Preserve the design pack's platform typography policy: Geist/Geist Mono on web, SF Pro/SF Mono on Darwin and Apple mobile, Roboto/Noto fallbacks on Android, Instrument Serif for editorial text, and Font Awesome for icons.
- Keep `lthn serve`, `lthn ai`, and other non-GUI commands independent from Wails window startup.
- Preserve `pkg/connection` as the transport boundary; do not replace the configurable WebSocket transport with WebView-only IPC.
- Do not publish transport access tokens in served JavaScript, persisted URLs, logs, or status output.
- Follow the CoreGO wrappers, `core.Result`, service shape, British-English copy, EUPL-1.2 licence, and TDD rules in `AGENTS.md`.
- Treat the external v0.9.0 audit as a before/after no-regression diagnostic, not an all-green prerequisite.
- Do not delete `frontend/` until desktop, iOS, and Android binding generators all write successfully to `frontend-ng/bindings/`.
- Do not delete `frontend-lit-ref/` until all 361 retired files are rechecked against `67b012f^:frontend`.
- Do not modify `/Users/snider/Downloads/Lethean-Desgin-Pack/`; it is read-only design reference material.
- App-by-app capability restoration and visual refinement are a follow-on programme. This plan records the capability baseline and preserves every Angular surface, but does not redesign all 66 registered apps in one change.

---

## Planned File Structure

New focused files:

- `scripts/repository.mjs` — small shared helpers for read-only Git file discovery and existence checks.
- `scripts/frontend-capability-inventory.mjs` — source-evidence inventory and Markdown renderer for every registered Angular app/surface.
- `scripts/frontend-capability-inventory.test.mjs` — parser, count, evidence, and non-overclaiming tests.
- `scripts/verify-frontend-build.mjs` — validates built font references and required font families.
- `scripts/verify-frontend-build.test.mjs` — temporary-fixture tests for missing and valid font assets.
- `scripts/verify-frontend-convergence.mjs` — rejects retired frontend paths and stale build assumptions.
- `scripts/verify-frontend-convergence.test.mjs` — repository-contract tests for binding targets, CI checkout, and retired paths.
- `frontend-ng/public/wails/transport.js` — harmless development-server fallback; Wails continues to intercept this reserved URL with its runtime-generated endpoint.
- `docs/frontend/capability-matrix.md` — generated source-evidence baseline, explicitly distinct from a runtime certification.
- `docs/design/README.md` — retained design provenance, production locations, and Git recovery points after archive removal.

Existing files changed in place:

- `frontend-ng/src/app/app.html` and its render/contract tests — CSR-only first paint with no hydration triggers.
- `frontend-ng/src/app/mobile-runtime.service.spec.ts` — desktop-native versus mobile presentation contract.
- `frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts` — Angular terminology and route count.
- `frontend-ng/package.json` — capability/build/convergence verification entrypoints.
- `build/Taskfile.yml` and all five files under `build/{darwin,linux,windows,ios,android}/Taskfile.yml` — workspace sync, canonical binding outputs, and public mobile binding verification tasks.
- `.github/workflows/build.yml`, `build/audit.sh`, and `Taskfile.yml` — current npm/Angular and no-submodule workflow.
- `README.md`, `CLAUDE.md`, `docs/development.md`, `docs/architecture.md`, and stale Go source comments — current paths and native/remote transport contract.
- `.gitignore` and `AGENTS.md` — post-retirement repository truth.
- Tracked archive files under `docs/design/lit/`, `docs/design/Lethean-5.zip`, `docs/design/HANDOVER.md`, and `frontend/bindings/` — removed only after their gates pass.

### Task 1: Generate an Honest Capability Evidence Baseline

**Files:**
- Create: `scripts/repository.mjs`
- Create: `scripts/frontend-capability-inventory.mjs`
- Create: `scripts/frontend-capability-inventory.test.mjs`
- Create: `docs/frontend/capability-matrix.md`
- Modify: `frontend-ng/package.json`
- Read: `frontend-ng/src/app/desktop/desktop.data.ts`
- Read: `frontend-ng/src/app/desktop/apps/app-view.ts`
- Read: `frontend-ng/src/app/desktop/surfaces/surface-registry.ts`
- Read: `frontend-ng/src/app/desktop/surfaces/**/*.ts`
- Read: `go/cmd/lthn/app.go`
- Read: `go/pkg/desktop/desktop.go`

**Interfaces:**
- Produces: `gitLines(repoRoot: string, args: string[]): Promise<string[]>` and `pathExists(path: string): Promise<boolean>` from `scripts/repository.mjs`.
- Produces: `collectCapabilityEvidence(repoRoot: string): Promise<CapabilityReport>`
- Produces: `renderCapabilityMatrix(report: CapabilityReport): string`
- Produces: `CapabilityReport` with `baseApps`, `surfaceApps`, and `entries`; each `CapabilityEntry` carries `contracts`, `evidence`, `limitations`, `resolvedContracts`, `specialisedEvidence`, and `sourceState`.
- Source-state values are exactly `integrated`, `unresolved`, and `design-fixture`; they are deliberately not the runtime maturity values `live`, `partial`, `dormant`, or `design-only`.
- Later app-family plans consume `docs/frontend/capability-matrix.md` and add runtime evidence before assigning runtime maturity.

- [ ] **Step 1: Write the failing inventory tests**

Create `scripts/frontend-capability-inventory.test.mjs` with real-repository assertions:

```js
import assert from 'node:assert/strict';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  collectCapabilityEvidence,
  renderCapabilityMatrix,
} from './frontend-capability-inventory.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

test('inventories every base app and routed surface without calling it live', async () => {
  const report = await collectCapabilityEvidence(repoRoot);

  assert.equal(report.baseApps.length, 23);
  assert.equal(report.surfaceApps.length, 43);
  assert.equal(report.surfaceApps.filter(({ contracts }) => contracts.length > 0).length, 35);
  assert.equal(new Set(report.surfaceApps.map(({ route }) => route)).size, 43);
  assert.equal(report.entries.some(({ sourceState }) => sourceState === 'live'), false);
});

test('renders evidence and limitations instead of mock-or-live guesses', async () => {
  const markdown = renderCapabilityMatrix(await collectCapabilityEvidence(repoRoot));

  assert.match(markdown, /^# Frontend Capability Matrix/m);
  assert.match(markdown, /Source state is not runtime maturity/);
  assert.match(markdown, /dappco\.re\/lthn\/desktop\/pkg\/tasks\.Service\.List/);
  assert.match(markdown, /\/v1\/ml-lab\/duckdb/);
  assert.doesNotMatch(markdown, /\| Live \|/);
});
```

- [ ] **Step 2: Run the inventory tests and confirm the missing-module failure**

Run:

```bash
node --test scripts/frontend-capability-inventory.test.mjs
```

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `scripts/frontend-capability-inventory.mjs`.

- [ ] **Step 3: Implement the source-evidence collector**

Create `scripts/repository.mjs` first:

```js
import { spawn } from 'node:child_process';
import { access } from 'node:fs/promises';

export function gitLines(repoRoot, args) {
  return new Promise((resolve, reject) => {
    const child = spawn('git', args, { cwd: repoRoot });
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => { stdout += chunk; });
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.on('error', reject);
    child.on('close', (code) => {
      if (code !== 0) {
        reject(new Error(`git ${args.join(' ')} failed: ${stderr.trim()}`));
        return;
      }
      resolve(stdout.split('\n').filter(Boolean));
    });
  });
}

export async function pathExists(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}
```

Then create `scripts/frontend-capability-inventory.mjs`. Import `readFile`,
`mkdir`, and `writeFile` from `node:fs/promises`, `dirname` and `join` from
`node:path`, and `gitLines`/`pathExists` from `./repository.mjs`. Keep parsing
limited to stable declarations: `APPS`, `SURFACE_APP_REGISTRY`,
route-group/route pairs, and literal `bridgeMethod`, `loadEndpoint`, or
`endpoint` values. Use these exact record shapes:

```js
/**
 * @typedef {'integrated'|'unresolved'|'design-fixture'} SourceState
 * @typedef {{
 *   id: string;
 *   route: string;
 *   component: string;
 *   contracts: string[];
 *   evidence: string[];
 *   limitations: string[];
 *   resolvedContracts: number;
 *   specialisedEvidence: string[];
 *   sourceState: SourceState;
 * }} CapabilityEntry
 * @typedef {{
 *   baseApps: CapabilityEntry[];
 *   surfaceApps: CapabilityEntry[];
 *   entries: CapabilityEntry[];
 * }} CapabilityReport
 */

export async function collectCapabilityEvidence(repoRoot) {
  const baseApps = await readBaseApps(repoRoot);
  const surfaceApps = await readSurfaceApps(repoRoot);
  const entries = [...baseApps, ...surfaceApps];

  for (const entry of entries) {
    const hasIntegration =
      entry.contracts.length > 0 || entry.specialisedEvidence.length > 0;
    const allContractsResolve =
      entry.resolvedContracts === entry.contracts.length;
    entry.sourceState = !hasIntegration
      ? 'design-fixture'
      : allContractsResolve && entry.evidence.length > 0
        ? 'integrated'
        : 'unresolved';
    entry.limitations.push('Runtime path not certified by this source audit.');
  }
  return { baseApps, surfaceApps, entries };
}
```

Use the complete base-component map rather than guessing filenames:

```js
const BASE_COMPONENTS = Object.freeze({
  control: 'frontend-ng/src/app/desktop/apps/control.app.ts',
  chat: 'frontend-ng/src/app/desktop/apps/chat.app.ts',
  telemetry: 'frontend-ng/src/app/desktop/apps/telemetry.app.ts',
  activity: 'frontend-ng/src/app/desktop/apps/activity.app.ts',
  lethernet: 'frontend-ng/src/app/desktop/apps/lethernet.app.ts',
  games: 'frontend-ng/src/app/desktop/apps/games.app.ts',
  notepad: 'frontend-ng/src/app/desktop/apps/notepad.app.ts',
  files: 'frontend-ng/src/app/desktop/apps/files.app.ts',
  settings: 'frontend-ng/src/app/desktop/apps/settings.app.ts',
  cpanel: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  explorer: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  codesearch: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  scm: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  terminal: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  build: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  procmon: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  containers: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  repos: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  forge: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  devops: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  marketplace: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  tasks: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
  tenant: 'frontend-ng/src/app/desktop/apps/dev-panel.app.ts',
});

const BASE_ROUTES = Object.freeze({
  control: '/system/control',
  telemetry: '/system/telemetry',
  activity: '/system/activity',
  settings: '/system/settings',
  cpanel: '/developer/control-panel',
  explorer: '/developer/explorer',
  codesearch: '/developer/search',
  scm: '/developer/git',
  build: '/developer/build',
  procmon: '/developer/process',
  containers: '/developer/containers',
  repos: '/developer/repos',
  forge: '/developer/forge',
  devops: '/developer/devops',
  marketplace: '/developer/marketplace',
  tasks: '/office/tasks',
  tenant: '/office/tenant',
  chat: '/ai/chat',
  games: '/media/games',
  files: '/tools/files',
  notepad: '/tools/notepad',
  terminal: '/tools/terminal',
  lethernet: '/networking/lethernet',
});

const SPECIALISED_EVIDENCE = Object.freeze({
  chat: ['frontend-ng/src/app/desktop/desktop-ai.service.ts'],
  settings: ['frontend-ng/src/app/desktop/preferences.service.ts'],
  'surface-agents-terminal': [
    'frontend-ng/src/app/desktop/surfaces/agents/terminal-session.ts',
  ],
  'surface-extensions-marketplace': [
    'frontend-ng/src/app/desktop/surfaces/extensions/marketplace.ts',
  ],
  'surface-extensions-plugin-view': [
    'frontend-ng/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts',
  ],
  'surface-extensions-opencode-shim': [
    'frontend-ng/src/app/desktop/surfaces/extensions/opencode-shim.ts',
  ],
});
```

Define the two readers with these exact parsing boundaries:

```js
async function readBaseApps(repoRoot) {
  const source = await readFile(
    join(repoRoot, 'frontend-ng/src/app/desktop/desktop.data.ts'),
    'utf8',
  );
  const block = source.slice(
    source.indexOf('export const APPS:'),
    source.indexOf('export const CATEGORIES:'),
  );
  const ids = [...block.matchAll(/^\s{2}([a-z][a-z0-9_-]+):\s*\{/gm)]
    .map((match) => match[1]);
  return Promise.all(ids.map((id) =>
    entryFromComponent(repoRoot, id, BASE_ROUTES[id], BASE_COMPONENTS[id]),
  ));
}

async function readSurfaceApps(repoRoot) {
  const source = await readFile(
    join(repoRoot, 'frontend-ng/src/app/desktop/surfaces/surface-registry.ts'),
    'utf8',
  );
  const block = source.slice(
    source.indexOf('const DEFINITIONS:'),
    source.indexOf('export function surfaceAppId'),
  );
  const definitions = [...block.matchAll(
    /group:\s*'([^']+)'[\s\S]*?route:\s*'([^']+)'[\s\S]*?title:/g,
  )];
  return Promise.all(definitions.map(([, group, route]) => {
    const id = `surface-${group}-${route}`;
    return entryFromComponent(
      repoRoot,
      id,
      `/${group}/${route}`,
      `frontend-ng/src/app/desktop/surfaces/${group}/${route}.ts`,
    );
  }));
}
```

`entryFromComponent` extracts only literal config contracts:

```js
const CONTRACT_PATTERN =
  /(?:bridgeMethod|loadEndpoint|endpoint):\s*['"`]([^'"`]+)['"`]/g;

async function entryFromComponent(repoRoot, id, route, component) {
  const source = await readFile(join(repoRoot, component), 'utf8');
  const contracts = [...source.matchAll(CONTRACT_PATTERN)].map((match) => match[1]);
  const specialisedEvidence = SPECIALISED_EVIDENCE[id] ?? [];
  const resolutions = await Promise.all(
    contracts.map((contract) => resolveContract(repoRoot, contract)),
  );
  const presentSpecialisedEvidence = [];
  for (const path of specialisedEvidence) {
    if (await pathExists(join(repoRoot, path))) presentSpecialisedEvidence.push(path);
  }
  return {
    id,
    route,
    component,
    contracts,
    evidence: [
      ...resolutions.flatMap(({ evidence }) => evidence),
      ...presentSpecialisedEvidence,
    ],
    limitations: [],
    resolvedContracts: resolutions.filter(({ resolved }) => resolved).length,
    specialisedEvidence,
    sourceState: 'unresolved',
  };
}
```

Define `resolveContract` exactly:

```js
async function resolveContract(repoRoot, contract) {
  const goFiles = (await gitLines(repoRoot, ['ls-files', 'go']))
    .filter((path) => path.endsWith('.go') && !path.endsWith('_test.go'));
  const method = contract.match(
    /^dappco\.re\/lthn\/desktop\/(pkg\/.+)\.Service\.([A-Za-z][A-Za-z0-9_]*)$/,
  );
  if (method) {
    const [, packagePath, methodName] = method;
    const candidates = goFiles.filter((path) =>
      path.startsWith(`go/${packagePath}/`),
    );
    const declaration = new RegExp(
      `func\\s*\\([^)]*\\*Service\\)\\s*${methodName}\\s*\\(`,
    );
    const evidence = [];
    for (const path of candidates) {
      const source = await readFile(join(repoRoot, path), 'utf8');
      if (declaration.test(source)) evidence.push(`${path}#Service.${methodName}`);
    }
    return { resolved: evidence.length > 0, evidence };
  }

  if (contract.startsWith('/')) {
    const evidence = [];
    for (const path of goFiles) {
      const source = await readFile(join(repoRoot, path), 'utf8');
      if (source.includes(contract)) evidence.push(`${path}#${contract}`);
    }
    return { resolved: evidence.length > 0, evidence };
  }

  return { resolved: false, evidence: [] };
}
```

Recognise specialised integration only from the explicit map above; do not
infer it from visual polish or fixture data. Record missing targets as
`unresolved`; do not delete their Angular source.

- [ ] **Step 4: Implement deterministic Markdown rendering and CLI output**

Add the renderer and CLI branch:

```js
export function renderCapabilityMatrix(report) {
  const rows = report.entries
    .toSorted((a, b) => a.route.localeCompare(b.route))
    .map((entry) => [
      entry.route,
      entry.component,
      entry.contracts.join('<br>') || 'none declared',
      entry.sourceState,
      entry.evidence.join('<br>') || 'component/route only',
      entry.limitations.join(' '),
    ]);

  return [
    '# Frontend Capability Matrix',
    '',
    '> Generated source evidence. Source state is not runtime maturity; promote a row to live only in an app-family plan with a passing runtime smoke.',
    '',
    '| Route | Component | Declared contract | Source state | Evidence | Limitation |',
    '|---|---|---|---|---|---|',
    ...rows.map((row) => `| ${row.join(' | ')} |`),
    '',
  ].join('\n');
}
```

When invoked as a command, accept `--write docs/frontend/capability-matrix.md`; otherwise print to stdout. Use `writeFile` only for that exact caller-supplied path.
Before writing, call `mkdir(dirname(outputPath), { recursive: true })`.
Use this CLI branch:

```js
const modulePath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(modulePath), '..');
const isMain =
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  const markdown = renderCapabilityMatrix(await collectCapabilityEvidence(repoRoot));
  const writeIndex = process.argv.indexOf('--write');
  if (writeIndex === -1) {
    process.stdout.write(markdown);
  } else {
    const requestedPath = process.argv[writeIndex + 1];
    if (!requestedPath) throw new Error('--write requires a repository-relative path');
    const outputPath = resolve(repoRoot, requestedPath);
    await mkdir(dirname(outputPath), { recursive: true });
    await writeFile(outputPath, markdown);
  }
}
```

Also import `fileURLToPath` and `pathToFileURL` from `node:url`, and `resolve`
from `node:path`.

- [ ] **Step 5: Add npm entrypoints and generate the matrix**

Add these scripts to `frontend-ng/package.json`:

```json
"test:contracts": "node --test ../scripts/*.test.mjs",
"audit:capabilities": "node ../scripts/frontend-capability-inventory.mjs --write ../docs/frontend/capability-matrix.md"
```

Run:

```bash
node --test scripts/frontend-capability-inventory.test.mjs
cd frontend-ng && npm run audit:capabilities
```

Expected: PASS, then a deterministic 66-row matrix with 23 base apps and 43 shared surfaces. The matrix must call fixture-only base apps `design-fixture`, not mock, dead, or live.

- [ ] **Step 6: Review every unresolved row before continuing**

Run:

```bash
rg -n '\| unresolved \|' docs/frontend/capability-matrix.md
rg -n 'Runtime path not certified' docs/frontend/capability-matrix.md
```

Expected: every unresolved contract has a concrete missing Go/binding/route evidence path; every row carries the runtime-certification limitation. Record repairs as follow-on app-family work, not archive-retirement blockers.

- [ ] **Step 7: Commit the baseline**

```bash
git add scripts/repository.mjs scripts/frontend-capability-inventory.mjs scripts/frontend-capability-inventory.test.mjs docs/frontend/capability-matrix.md frontend-ng/package.json
git commit -m "docs(frontend): inventory Angular capability evidence"
```

### Task 2: Pin the CSR Router and Shared Presentation Contract

**Files:**
- Modify: `frontend-ng/src/app/app.html`
- Modify: `frontend-ng/src/app/app.render.spec.ts`
- Modify: `frontend-ng/src/app/mobile-runtime.service.spec.ts`
- Modify: `frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts`
- Modify: `scripts/verify-frontend-convergence.test.mjs` when created in this task
- Create: `scripts/verify-frontend-convergence.test.mjs`

**Interfaces:**
- Consumes: existing `routes`, `WindowManagerService`, `MobileRuntimeService`, and `HashLocationStrategy`.
- Produces: a CSR-only root template using `@defer (on immediate)` without hydrate triggers.
- Produces: tests proving desktop-native platforms retain the selected desktop/shell view while iOS/iPad/Android select device presentation.

- [ ] **Step 1: Write a failing no-hydration source contract**

Create `scripts/verify-frontend-convergence.test.mjs`:

```js
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const read = (path) => readFile(`${repoRoot}/${path}`, 'utf8');

test('keeps the Angular root client-rendered without hydration triggers', async () => {
  const template = await read('frontend-ng/src/app/app.html');
  assert.match(template, /@defer \(on immediate\)/);
  assert.doesNotMatch(template, /hydrate\s+on/);
  assert.match(template, /<router-outlet\s*\/>/);
});
```

- [ ] **Step 2: Run the source contract and confirm it fails on the current template**

Run:

```bash
node --test scripts/verify-frontend-convergence.test.mjs
```

Expected: FAIL because `app.html` contains `hydrate on interaction` and `hydrate on timer`.

- [ ] **Step 3: Remove the unsupported hydration triggers**

Replace `frontend-ng/src/app/app.html` with:

```html
@defer (on immediate) {
  <router-outlet />
} @placeholder {
  <app-app-shell />
}
```

This preserves the immediate route-outlet first paint and its app-shell placeholder while matching the repository's CSR-only architecture.

- [ ] **Step 4: Extend first-paint assertions**

In `frontend-ng/src/app/app.render.spec.ts`, keep the existing `#/` and `#/system/telemetry` assertions and add:

```ts
expect(element.querySelector('app-app-shell')).toBeNull();
expect(element.querySelector('router-outlet')).not.toBeNull();
```

Expected result after `vi.waitFor`: the routed desktop owns the settled render and the placeholder does not become a second application tree.

- [ ] **Step 5: Add desktop-native versus mobile platform cases**

Refactor the mobile-runtime test setup into `createService(platform: LetheanPlatform)` and add:

```ts
const createService = (platform: LetheanPlatform): MobileRuntimeService => {
  listeners = new Map();
  emit = vi.fn((_name: string, _payload: Record<string, unknown>) => Promise.resolve());
  const transport: MobileRuntimeTransport = {
    platform: () => platform,
    on(name, handler): () => void {
      listeners.set(name, handler);
      return () => listeners.delete(name);
    },
    emit(name, payload): Promise<void> {
      return emit(name, payload);
    },
  };
  windows = {
    setView: vi.fn(),
    setDevice: vi.fn(),
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MOBILE_RUNTIME_TRANSPORT, useValue: transport },
      { provide: WindowManagerService, useValue: windows },
    ],
  });
  return TestBed.inject(MobileRuntimeService);
};

it.each(['darwin', 'windows', 'linux'] as const)(
  'marks %s as native desktop without forcing device presentation',
  async (platform) => {
    service.destroy();
    TestBed.resetTestingModule();
    service = createService(platform);
    await service.ready;

    expect(document.documentElement.dataset['platform']).toBe(platform);
    expect(windows.setView).not.toHaveBeenCalled();
    expect(windows.setDevice).not.toHaveBeenCalled();
  },
);
```

Import `LetheanPlatform` from `mobile-runtime.service`. In `beforeEach`, set
`service = createService('ios')` and await `service.ready`. Keep the existing
iOS assertions and add iPad/Android cases asserting `setView('device')`, with
iPad selecting `large`.

- [ ] **Step 6: Correct stale Lit terminology**

Change the surface registry test name from:

```ts
it('registers all 43 Lit surface routes exactly once', () => {
```

to:

```ts
it('registers all 43 Angular surface routes exactly once', () => {
```

The portable elements remain Lit; the routed surface components are Angular.

- [ ] **Step 7: Run focused and complete frontend tests**

Run:

```bash
cd frontend-ng
npx ng test --watch=false --include=src/app/app.render.spec.ts
npx ng test --watch=false --include=src/app/mobile-runtime.service.spec.ts
npx ng test --watch=false --include=src/app/desktop/surfaces/surface-registry.spec.ts
npm run test:ci
```

Expected: all focused specs and the full frontend suite pass; a browser development smoke no longer reports Angular `NG0508`.

- [ ] **Step 8: Commit the presentation contract**

```bash
git add frontend-ng/src/app/app.html frontend-ng/src/app/app.render.spec.ts frontend-ng/src/app/mobile-runtime.service.spec.ts frontend-ng/src/app/desktop/surfaces/surface-registry.spec.ts scripts/verify-frontend-convergence.test.mjs
git commit -m "fix(frontend): keep the shared shell client rendered"
```

### Task 3: Verify Native/Remote Bootstrap and Font Assets

**Files:**
- Create: `frontend-ng/public/wails/transport.js`
- Create: `scripts/verify-frontend-build.mjs`
- Create: `scripts/verify-frontend-build.test.mjs`
- Modify: `frontend-ng/package.json`
- Modify: `frontend-ng/src/app/connection-manager.service.spec.ts`
- Modify: `go/pkg/connection/transport_internal_test.go`
- Read: `frontend-ng/src/foundations/tokens/fonts.scss`
- Read: `frontend-ng/src/foundations/tokens/platforms.scss`

**Interfaces:**
- Consumes: `globalThis.__LTHN_CONNECTION__.webSocketUrl`, `ConnectionManagerService`, and Wails' reserved `/wails/transport.js`.
- Produces: `verifyFrontendBuild(distDir: string): Promise<FontBuildReport>`.
- Produces: `FontBuildReport` with `requiredFamilies: string[]`, `references: string[]`, and `missingAssets: string[]`.
- Produces: a development fallback endpoint of `ws://localhost:9099/wails/ws`; Wails' `application.App` still serves the environment-derived `JSClient()` at the same reserved path in native/served mode.
- Produces: build verification that every CSS-referenced font asset exists and the four required families are declared.

- [ ] **Step 1: Write failing fallback and font-verifier tests**

Create `scripts/verify-frontend-build.test.mjs`:

```js
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runInNewContext } from 'node:vm';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { verifyFrontendBuild } from './verify-frontend-build.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

test('development transport bootstrap publishes the loopback default only', async () => {
  const source = await readFile(`${repoRoot}/frontend-ng/public/wails/transport.js`, 'utf8');
  const sandbox = { globalThis: {} };
  runInNewContext(source, sandbox);
  assert.deepEqual(
    JSON.parse(JSON.stringify(sandbox.globalThis.__LTHN_CONNECTION__)),
    { webSocketUrl: 'ws://localhost:9099/wails/ws' },
  );
  assert.equal(Object.isFrozen(sandbox.globalThis.__LTHN_CONNECTION__), true);
});

test('font verification rejects a referenced asset that is absent', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(
    join(root, 'styles.css'),
    '@font-face{font-family:"Geist";src:url("./media/geist.woff2")}\n' +
      '@font-face{font-family:"Geist Mono";src:url("./media/mono.woff2")}\n' +
      '@font-face{font-family:"Instrument Serif";src:url("./media/serif.woff2")}\n' +
      '@font-face{font-family:"Font Awesome 7 Free";src:url("./media/icons.woff2")}',
  );
  await assert.rejects(verifyFrontendBuild(root), /missing font asset/);
});

test('font verification accepts complete required families', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await mkdir(join(root, 'media'));
  for (const name of ['geist', 'mono', 'serif', 'icons']) {
    await writeFile(join(root, 'media', `${name}.woff2`), name);
  }
  await writeFile(
    join(root, 'styles.css'),
    [
      ['Geist', 'geist'],
      ['Geist Mono', 'mono'],
      ['Instrument Serif', 'serif'],
      ['Font Awesome 7 Free', 'icons'],
    ].map(([family, file]) =>
      `@font-face{font-family:"${family}";src:url("./media/${file}.woff2")}`,
    ).join('\n'),
  );

  const report = await verifyFrontendBuild(root);
  assert.deepEqual(report.requiredFamilies, [
    'Geist',
    'Geist Mono',
    'Instrument Serif',
    'Font Awesome 7 Free',
  ]);
  assert.equal(report.missingAssets.length, 0);
});
```

- [ ] **Step 2: Run the tests and confirm both new files are missing**

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
```

Expected: FAIL because the fallback and verifier do not exist.

- [ ] **Step 3: Add the ng-serve fallback without changing native Wails interception**

Create `frontend-ng/public/wails/transport.js`:

```js
globalThis.__LTHN_CONNECTION__ = Object.freeze({
  webSocketUrl: 'ws://localhost:9099/wails/ws',
});
```

Angular's public asset copier serves this during `ng serve`. In Wails, `/wails/transport.js` remains a reserved application path and `pkg/connection.Service.jsClient()` supplies the configured public URL.

- [ ] **Step 4: Implement the build verifier**

Create `scripts/verify-frontend-build.mjs` with these constants and checks:

```js
const REQUIRED_FAMILIES = [
  'Geist',
  'Geist Mono',
  'Instrument Serif',
  'Font Awesome 7 Free',
];

export async function verifyFrontendBuild(distDir) {
  const cssFiles = (await readdir(distDir)).filter((name) => name.endsWith('.css'));
  const css = (await Promise.all(
    cssFiles.map((name) => readFile(join(distDir, name), 'utf8')),
  )).join('\n');
  const references = [...css.matchAll(/url\(["']?(\.\/media\/[^)"']+\.(?:woff2?|ttf|otf))/g)]
    .map((match) => match[1]);
  const missingAssets = [];

  for (const reference of new Set(references)) {
    const path = join(distDir, reference.replace(/^\.\//, ''));
    try {
      await access(path);
    } catch {
      missingAssets.push(reference);
    }
  }
  if (missingAssets.length > 0) {
    throw new Error(`missing font asset: ${missingAssets.join(', ')}`);
  }
  for (const family of REQUIRED_FAMILIES) {
    const escaped = family.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const declaration = new RegExp(
      `font-family:\\s*["']?${escaped}["']?(?:;|})`,
    );
    if (!declaration.test(css)) {
      throw new Error(`missing required font family: ${family}`);
    }
  }
  return { requiredFamilies: REQUIRED_FAMILIES, references, missingAssets };
}
```

Import `access`, `readdir`, and `readFile` from `node:fs/promises`, and `join` from `node:path`. Add a CLI branch that verifies the supplied dist directory and prints only the family count, reference count, and success verdict.

Use:

```js
const isMain =
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  const requestedPath = process.argv[2];
  if (!requestedPath) throw new Error('usage: verify-frontend-build.mjs DIST_DIR');
  const report = await verifyFrontendBuild(resolve(requestedPath));
  console.log(
    `frontend font build: ${report.requiredFamilies.length} families, ` +
      `${report.references.length} references, PASS`,
  );
}
```

Import `pathToFileURL` from `node:url` and `resolve` from `node:path`.

- [ ] **Step 5: Pin remote transport precedence and token secrecy**

Extend `frontend-ng/src/app/connection-manager.service.spec.ts` with a case where:

```ts
configure(
  {
    protocol: 'https:',
    host: 'ui.lethean.example',
    search: '?lthn-ws=wss%3A%2F%2Fapi.lethean.example%2Fwails%2Fws',
  },
  {},
);
await service.ready;

expect(service.url()).toBe('wss://api.lethean.example/wails/ws');
expect(socketURLs).toEqual(['wss://api.lethean.example/wails/ws']);
```

Keep the existing tests proving explicit options, query/runtime configuration, safe persistence, same-origin proxy conversion, reconnects, request bounds, and insecure-remote rejection.

- [ ] **Step 6: Pin the native bootstrap payload on the Go side**

Extend `go/pkg/connection/transport_internal_test.go`:

```go
func TestConnection_Transport_Good_PublishesConfiguredClientURL(t *core.T) {
	svc := NewService(Options{
		PublicURL: "wss://api.lethean.example/wails/ws",
		Token:     "never-publish-this-secret",
	})
	client := string(svc.jsClient())

	core.AssertContains(t, client, `globalThis.__LTHN_CONNECTION__=Object.freeze(`)
	core.AssertContains(t, client, `"webSocketUrl":"wss://api.lethean.example/wails/ws"`)
	core.AssertFalse(t, core.Contains(client, "never-publish-this-secret"))
}
```

This complements the existing Wails option test which asserts the supplied `application.Transport` is preserved.

- [ ] **Step 7: Add build verification entrypoints and run them**

Add to `frontend-ng/package.json`:

```json
"verify:build": "node ../scripts/verify-frontend-build.mjs ../go/cmd/lthn/dist"
```

Run:

```bash
node --test scripts/verify-frontend-build.test.mjs
go test ./go/pkg/connection ./go/pkg/desktop
cd frontend-ng
npm run build
npm run verify:build
```

Expected: all tests pass; production CSS declares all four families; every referenced font file exists under `go/cmd/lthn/dist/media/`.

- [ ] **Step 8: Smoke browser typography and host bootstrap**

Run:

```bash
cd frontend-ng
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
```

In the browser console, verify:

```js
document.documentElement.dataset.platform
document.fonts.check('15px Geist')
document.fonts.check('12px "Geist Mono"')
await document.fonts.load('italic 16px "Instrument Serif"')
```

Expected: `web`, `true`, `true`, and one loaded Instrument Serif face. Network requests for `/wails/transport.js` and each requested font return 200; a closed backend may leave the WebSocket disconnected, which is an honest offline state.

- [ ] **Step 9: Smoke the native host before changing font policy**

Run from the repository root:

```bash
wails3 task dev
```

Expected on macOS: a normal Wails native window, `data-platform="darwin"`, computed UI font beginning with `-apple-system`/SF Pro, computed mono font beginning with SF Mono, Instrument Serif available on explicit load, Font Awesome icons rendered, and no `media/*` request receiving `index.html`. On Windows CI or a Windows workstation, the equivalent host is WebView2 and must load the same Angular dist. If an embedded request fails, stop this task and record its URL, status, response MIME, CSP message, and whether the body is `index.html`; retirement does not proceed until a focused asset-routing defect plan passes this same smoke. Do not remove the pack's intentional Darwin font override.

- [ ] **Step 10: Commit native/remote bootstrap verification**

```bash
git add frontend-ng/public/wails/transport.js scripts/verify-frontend-build.mjs scripts/verify-frontend-build.test.mjs frontend-ng/package.json frontend-ng/src/app/connection-manager.service.spec.ts go/pkg/connection/transport_internal_test.go
git commit -m "test(frontend): verify native transport and font assets"
```

### Task 4: Repair Binding Generation and CI Plumbing

**Files:**
- Modify: `scripts/verify-frontend-convergence.test.mjs`
- Modify: `build/Taskfile.yml`
- Modify: `build/darwin/Taskfile.yml`
- Modify: `build/linux/Taskfile.yml`
- Modify: `build/windows/Taskfile.yml`
- Modify: `build/ios/Taskfile.yml`
- Modify: `build/android/Taskfile.yml`
- Modify: `.github/workflows/build.yml`
- Modify: `build/audit.sh`
- Modify: `Taskfile.yml`

**Interfaces:**
- Produces: desktop, iOS, and Android binding generators whose only TypeScript destination is `frontend-ng/bindings/`.
- Produces: public `ios:bindings` and `android:bindings` verification tasks.
- Produces: CI checkout with no recursive-submodule assumption.
- Produces: an audit path using Angular/npm and the current no-regression audit policy.

- [ ] **Step 0: Capture the pre-change CoreGO diagnostic**

Run:

```bash
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh .
```

Record the eight code-wrongness counts listed in Step 7 in the task execution
log before editing files. This read-only output is the baseline consumed by
Task 8; do not turn the full historical backlog into this task's scope.

- [ ] **Step 1: Add failing repository-plumbing contracts**

Append to `scripts/verify-frontend-convergence.test.mjs`:

```js
test('all binding generators target frontend-ng and no removed external tree', async () => {
  const files = await Promise.all([
    read('build/Taskfile.yml'),
    read('build/ios/Taskfile.yml'),
    read('build/android/Taskfile.yml'),
  ]);
  const combined = files.join('\n');

  assert.doesNotMatch(combined, /frontend\/bindings/);
  assert.doesNotMatch(combined, /external\/gui/);
  assert.match(combined, /frontend-ng\/bindings/);
});

test('CI and audit use the current module and Angular topology', async () => {
  const workflow = await read('.github/workflows/build.yml');
  const audit = await read('build/audit.sh');

  assert.doesNotMatch(workflow, /submodules:\s*recursive/);
  assert.match(audit, /cd frontend-ng/);
  assert.match(audit, /npm run build/);
  assert.match(audit, /npm run test:ci/);
  assert.doesNotMatch(audit, /bun run|cd frontend(?:\s|$)/);
});
```

- [ ] **Step 2: Run the contracts and confirm all known drift is detected**

Run:

```bash
node --test scripts/verify-frontend-convergence.test.mjs
```

Expected: FAIL on `frontend/bindings`, `external/gui`, recursive submodules, and Bun.

- [ ] **Step 3: Repair the common desktop generator and workspace sync**

In `build/Taskfile.yml`, replace the no-op task with:

```yaml
  go:work:sync:
    summary: Synchronise the ./go module selected by the root go.work
    internal: true
    cmds:
      - go work sync
```

Change `generate:bindings` to depend on `go:work:sync`, keep `dir: "{{.GO_DIR}}"`, and use:

```yaml
    generates:
      - ../frontend-ng/bindings/**/*
    cmds:
      - wails3 generate bindings -ts -d ../frontend-ng/bindings -f '{{.BUILD_FLAGS}}' -clean=true ./pkg/desktop/...
```

Rename `common:go:mod:tidy` to `common:go:work:sync` in:

```text
build/darwin/Taskfile.yml
build/linux/Taskfile.yml
build/windows/Taskfile.yml
build/ios/Taskfile.yml
build/android/Taskfile.yml
```

- [ ] **Step 4: Repair and expose the iOS binding task**

In `build/ios/Taskfile.yml`, add:

```yaml
  bindings:
    summary: Generate iOS TypeScript bindings into frontend-ng/bindings
    cmds:
      - task: generate:ios:bindings
```

Change its `generates` declaration to:

```yaml
    generates:
      - frontend-ng/bindings/**/*
```

Keep the existing command destination `../frontend-ng/bindings`.

- [ ] **Step 5: Repair and expose the Android binding task**

In `build/android/Taskfile.yml`, add:

```yaml
  bindings:
    summary: Generate Android TypeScript bindings into frontend-ng/bindings
    cmds:
      - task: generate:android:bindings
```

Change its `generates` declaration to:

```yaml
    generates:
      - frontend-ng/bindings/**/*
```

Keep the existing `GOOS=android`, `CGO_ENABLED=0`, `-tags android`, and `../frontend-ng/bindings` command.

- [ ] **Step 6: Remove the obsolete CI submodule checkout**

Reduce the checkout step in `.github/workflows/build.yml` to:

```yaml
      - name: Checkout
        uses: actions/checkout@v4
```

Do not add a replacement dependency fetch; `go.work`, `go/go.mod`, and `go/go.sum` are authoritative.

- [ ] **Step 7: Make the audit target the current frontend and report audit debt honestly**

Change the frontend steps in `build/audit.sh` to:

```bash
step "frontend build"  bash -c 'cd frontend-ng && npm run build'
step "frontend test"   bash -c 'cd frontend-ng && npm run test:ci'
step "frontend contracts" bash -c 'cd frontend-ng && npm run test:contracts'
```

Change `audit_v090` so a non-compliant existing backlog is an explicit
diagnostic warning rather than a false clean result or an unconditional stop:

```bash
  printf '⚠ v0.9.0 compliance — non-compliant baseline (diagnostic)\n'
  echo "$out" | grep -E '^\s+[a-z][a-z-]+\s+[1-9]|verdict:' || true
  return 0
```

Keep the full current backlog visible in verbose mode; never print a false
`COMPLIANT`. Task 8 performs the explicit before/after no-regression comparison
for these eight code-wrongness dimensions:

```text
banned-imports
err-shape-funcs
tuple-result-shape
result-discards
service-canonical-shape
service-usage-example
service-name-empty
legacy-imports
```

- [ ] **Step 8: Include contract verification in the root frontend task**

Change `Taskfile.yml`:

```yaml
  test:frontend:
    summary: Run Angular Vitest plus frontend convergence contracts
    cmds:
      - cd frontend-ng && npm run test:ci
      - cd frontend-ng && npm run test:contracts
```

- [ ] **Step 9: Regenerate every binding flavour**

Run in this order:

```bash
go work sync
wails3 task common:generate:bindings
wails3 task ios:bindings
wails3 task android:bindings
wails3 task common:generate:bindings
```

Expected: every command succeeds, every generated file is under ignored `frontend-ng/bindings/`, no command reads `external/gui`, and the final desktop regeneration restores the normal host binding set.

- [ ] **Step 10: Re-run plumbing contracts and frontend typecheck**

Run:

```bash
node --test scripts/verify-frontend-convergence.test.mjs
cd frontend-ng && npx tsc --noEmit
```

Expected: PASS.

- [ ] **Step 11: Commit the pipeline repair**

```bash
git add scripts/verify-frontend-convergence.test.mjs build/Taskfile.yml build/darwin/Taskfile.yml build/linux/Taskfile.yml build/windows/Taskfile.yml build/ios/Taskfile.yml build/android/Taskfile.yml .github/workflows/build.yml build/audit.sh Taskfile.yml
git commit -m "fix(build): converge bindings on the Angular frontend"
```

### Task 5: Replace Stale Paths in Documentation and Source Comments

**Files:**
- Modify: `scripts/verify-frontend-convergence.mjs`
- Modify: `scripts/verify-frontend-convergence.test.mjs`
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/development.md`
- Modify: `docs/architecture.md`
- Modify: Go files returned by the scoped search below

**Interfaces:**
- Produces: `verifyFrontendConvergence(repoRoot: string): Promise<ConvergenceReport>`.
- Produces: `ConvergenceReport` with `staleReferences: Array<{ path: string; pattern: string }>`.
- Produces: a checked allow-list for historical references inside the approved spec/plan and recovery documentation.
- Keeps comments behaviour-neutral; this task changes no Go signature, route, service, or binding.

- [ ] **Step 1: Add failing stale-path tests**

Add this top-level import:

```js
import { verifyFrontendConvergence } from './verify-frontend-convergence.mjs';
```

Then append:

```js
test('rejects active references to retired frontend and submodule topology', async () => {
  const report = await verifyFrontendConvergence(repoRoot);
  assert.deepEqual(report.staleReferences, []);
});
```

The verifier must search tracked text files while excluding:

```js
const HISTORICAL_REFERENCE_ALLOWLIST = [
  'AGENTS.md',
  'docs/superpowers/specs/2026-07-25-frontend-convergence-design.md',
  'docs/superpowers/plans/2026-07-25-frontend-convergence.md',
  'docs/design/README.md',
];
```

Reject active occurrences of:

```js
const RETIRED_PATTERNS = [
  'frontend/src/lit',
  'frontend/bindings',
  'frontend-lit-ref',
  'docs/design/Lethean-5.zip',
  'docs/design/lit/',
  'external/gui',
  './external/*',
  'lethean-4-react-reference',
  'git submodule update --init --recursive',
  'submodules: recursive',
  'bun run',
];
```

- [ ] **Step 2: Run the test and capture the exact stale file list**

Run:

```bash
node --test scripts/verify-frontend-convergence.test.mjs
rg -l 'frontend/src/lit|frontend/bindings|external/gui|submodules: recursive|bun run' \
  README.md CLAUDE.md docs go build .github Taskfile.yml \
  --glob '!go/cmd/lthn/dist/**' \
  --glob '!docs/superpowers/specs/2026-07-25-frontend-convergence-design.md' \
  --glob '!docs/superpowers/plans/2026-07-25-frontend-convergence.md'
```

Expected: FAIL and a finite list matching the known migration drift.

- [ ] **Step 3: Implement the tracked-text verifier**

Create `scripts/verify-frontend-convergence.mjs` using:

```js
export async function verifyFrontendConvergence(repoRoot) {
  const tracked = await gitLines(repoRoot, ['ls-files']);
  const candidates = tracked.filter(isTextPath).filter(
    (path) => !HISTORICAL_REFERENCE_ALLOWLIST.includes(path),
  );
  const staleReferences = [];
  for (const path of candidates) {
    const source = await readFile(join(repoRoot, path), 'utf8');
    for (const pattern of RETIRED_PATTERNS) {
      if (source.includes(pattern)) staleReferences.push({ path, pattern });
    }
  }
  return { staleReferences };
}
```

Import the shared Git reader and define the text-path filter:

```js
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { gitLines } from './repository.mjs';

function isTextPath(path) {
  return /\.(?:go|md|mjs|js|ts|html|scss|json|ya?ml|sh)$/.test(path) ||
    /(?:^|\/)(?:Taskfile|Dockerfile)(?:\.|$)/.test(path);
}
```

The CLI exits non-zero and prints `path: pattern` for each finding.
Use:

```js
const modulePath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(modulePath), '..');
const isMain =
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  const report = await verifyFrontendConvergence(repoRoot);
  for (const finding of report.staleReferences) {
    console.error(`${finding.path}: ${finding.pattern}`);
  }
  if (report.staleReferences.length > 0) process.exitCode = 1;
}
```

Add `dirname` and `resolve` to the `node:path` import, plus `fileURLToPath` and
`pathToFileURL` from `node:url`.

- [ ] **Step 4: Correct the primary documentation**

Make these exact truths consistent across `README.md`, `CLAUDE.md`, `docs/development.md`, and `docs/architecture.md`:

```text
go.work uses ./go only.
There is no .gitmodules or external/ checkout.
frontend-ng is Angular 22 CSR and builds to go/cmd/lthn/dist.
Wails hosts native windows; WebView2 is the Windows host engine.
ConnectionManagerService may connect over ws/wss independently of the WebView.
lthn serve and lthn ai do not construct a Wails application.
npm ci, npm run build, and npm run test:ci are the frontend commands.
```

Remove “when wired” language for already implemented GUI/server paths.

- [ ] **Step 5: Correct stale Go comments mechanically**

For each Go file reported by the scoped `rg`, replace:

```text
frontend/src/lit/...      -> the matching frontend-ng/src/app/... surface
frontend/bindings        -> frontend-ng/bindings
external/gui             -> the versioned dappco.re/go/render/display/webkit module
```

Do not alter imports, function bodies, route names, or generated binding identifiers.

- [ ] **Step 6: Verify comments and docs without formatting churn**

Run:

```bash
node --test scripts/verify-frontend-convergence.test.mjs
node scripts/verify-frontend-convergence.mjs
gofmt -l go/
git diff --check
```

Expected: both convergence checks pass, `gofmt -l` reports no newly changed Go formatting, and `git diff --check` is clean.

- [ ] **Step 7: Run focused Go documentation-adjacent tests**

Run:

```bash
go test ./go/cmd/lthn ./go/pkg/desktop ./go/pkg/connection ./go/pkg/server
```

Expected: PASS; comment-only edits introduce no behaviour change.

- [ ] **Step 8: Commit the path convergence**

```bash
git add scripts/verify-frontend-convergence.mjs scripts/verify-frontend-convergence.test.mjs README.md CLAUDE.md docs/development.md docs/architecture.md go
git commit -m "docs: replace retired frontend and workspace paths"
```

### Task 6: Retire Duplicate Frontend Archives with Recovery Proof

**Files:**
- Create: `docs/design/README.md`
- Modify: `.gitignore`
- Modify: `AGENTS.md`
- Modify: `scripts/verify-frontend-convergence.mjs`
- Delete: `docs/design/HANDOVER.md`
- Delete: `docs/design/Lethean-5.zip`
- Delete: `docs/design/lit/HANDOVER.md`
- Delete: `docs/design/lit/lit-chat-window.js`
- Delete: `docs/design/lit/lit-chrome.js`
- Delete: `docs/design/lit/lit-desktop.html`
- Delete: `docs/design/lit/lit-ext-windows.js`
- Delete: `docs/design/lit/lit-obs-windows.js`
- Delete: `docs/design/lit/lit-ops-windows.js`
- Delete: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventcreate.js`
- Delete: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts`
- Move to macOS Trash after verification: `frontend-lit-ref/`

**Interfaces:**
- Consumes: successful Task 4 desktop/iOS/Android binding regeneration.
- Produces: one product frontend in the working tree.
- Produces: recovery references `67b012f^`, `412a479`, and `afb79be`.
- Preserves: `/Users/snider/Downloads/Lethean-Desgin-Pack/` unchanged.

- [ ] **Step 1: Verify the old application snapshot file-for-file**

Run:

```bash
test "$(git ls-tree -r --name-only 67b012f^:frontend | wc -l | tr -d ' ')" = "361"
test "$(find frontend-lit-ref/frontend -type f | wc -l | tr -d ' ')" = "361"
mismatches=0
while read -r mode object_type expected_blob path; do
  actual_blob="$(git hash-object "frontend-lit-ref/frontend/$path")"
  if [ "$actual_blob" != "$expected_blob" ]; then
    echo "mismatch: $path"
    mismatches=$((mismatches + 1))
  fi
done < <(git ls-tree -r 67b012f^:frontend)
test "$mismatches" -eq 0
```

Expected: 361 matches and zero mismatches. Stop this task on any mismatch.

- [ ] **Step 2: Verify the tracked design archive provenance**

Run:

```bash
git log --oneline -- docs/design/HANDOVER.md docs/design/Lethean-5.zip docs/design/lit
git show --stat --oneline 412a479
git show --stat --oneline afb79be
```

Expected: the Lit/design archive is recoverable from `412a479` and `afb79be`. Confirm the attached design pack remains present and unmodified:

```bash
test -d /Users/snider/Downloads/Lethean-Desgin-Pack
```

- [ ] **Step 3: Verify mobile and desktop generators no longer need top-level frontend**

Run:

```bash
wails3 task common:generate:bindings
wails3 task ios:bindings
wails3 task android:bindings
rg -n 'frontend/bindings' build Taskfile.yml .github scripts \
  --glob '!docs/superpowers/**'
```

Expected: all three generators succeed and `rg` returns no active reference.

- [ ] **Step 4: Write the retained design provenance**

Create `docs/design/README.md` with:

```markdown
# Lethean Desktop Design Provenance

The Lethean Design Pack is the visual upstream for Lethean Desktop. Production
tokens live in `frontend-ng/src/foundations/`, reusable Lit elements live in
`frontend-ng/src/kit/`, and the Angular desktop/app-shell implementation lives
in `frontend-ng/src/app/desktop/`.

The former whole-application Lit snapshot is recoverable from
`67b012f^:frontend`. The in-repository Lit canvas and Lethean-5 archive are
recoverable from commits `412a479` and `afb79be`. Git is the archive; duplicate
working implementations are not retained.

When applying a new design pack, port reviewed token, element, asset, and
interaction changes into the production paths above. Do not restore a second
product frontend.
```

- [ ] **Step 5: Delete tracked archives and generated remnants with `apply_patch`**

Delete exactly the files listed in this task's **Files** section. Remove the now-empty tracked `frontend/` and `docs/design/lit/` directory trees as a consequence; do not touch `frontend-ng/`.

- [ ] **Step 6: Move the verified ignored snapshot to Trash**

Use an explicit recoverable target:

```bash
test -d frontend-lit-ref/frontend
test ! -e /Users/snider/.Trash/lthn-desktop-frontend-lit-ref-retired
mv frontend-lit-ref /Users/snider/.Trash/lthn-desktop-frontend-lit-ref-retired
```

The snapshot remains recoverable both from Trash and from `67b012f^`.

- [ ] **Step 7: Remove retirement-only ignore/docs text**

Remove `frontend-lit-ref/` from `.gitignore`. Update `AGENTS.md` so its
directory map no longer lists `frontend/`, `frontend-lit-ref/`, or tracked
`docs/design/lit/`; retain the recovery commits and the rule that Lit inside
`frontend-ng/src/kit/` is intentional. Remove `AGENTS.md` from
`HISTORICAL_REFERENCE_ALLOWLIST` in
`scripts/verify-frontend-convergence.mjs` after those current-state notes have
been rewritten.

- [ ] **Step 8: Verify the working tree now has one product frontend**

Run:

```bash
test -d frontend-ng
test ! -e frontend
test ! -e frontend-lit-ref
test ! -e docs/design/lit
test ! -e docs/design/Lethean-5.zip
git ls-files frontend frontend-lit-ref docs/design/lit docs/design/Lethean-5.zip docs/design/HANDOVER.md
node scripts/verify-frontend-convergence.mjs
git diff --check
```

Expected: all removed paths are absent; `git ls-files` prints nothing for them; the convergence verifier and diff check pass.

- [ ] **Step 9: Commit the recoverable retirement**

```bash
git add .gitignore AGENTS.md docs/design/README.md
git add -A docs/design frontend
git commit -m "chore(frontend): retire recoverable UI archives"
```

Record in the commit body:

```text
Old application: 67b012f^:frontend (361/361 blobs verified)
Design canvas/archive: 412a479 and afb79be
Canonical product frontend: frontend-ng/
```

### Task 7: Wire Guardrails into the Normal Gates

**Files:**
- Modify: `frontend-ng/package.json`
- Modify: `Taskfile.yml`
- Modify: `.github/workflows/build.yml`
- Modify: `scripts/verify-frontend-convergence.test.mjs`

**Interfaces:**
- Consumes: `audit:capabilities`, `verify:build`, `test:contracts`, and the Angular/Go suites.
- Produces: normal local and CI gates that reject a second frontend, stale binding paths, missing font assets, route-registry drift, and reintroduced hydration.

- [ ] **Step 1: Add a failing post-retirement repository-shape contract**

Append:

```js
test('keeps exactly one product frontend in the working tree', async () => {
  const tracked = await gitLines(repoRoot, ['ls-files']);
  assert.equal(tracked.some((path) => path.startsWith('frontend/')), false);
  assert.equal(tracked.some((path) => path.startsWith('frontend-lit-ref/')), false);
  assert.equal(tracked.some((path) => path.startsWith('docs/design/lit/')), false);
  assert.equal(tracked.includes('docs/design/Lethean-5.zip'), false);
  assert.equal(tracked.some((path) => path.startsWith('frontend-ng/src/')), true);
  assert.equal(tracked.some((path) => path.startsWith('frontend-ng/src/kit/')), true);
});
```

Add a second top-level import so the test uses the same non-shell Git reader as
the verifier:

```js
import { gitLines } from './repository.mjs';
```

- [ ] **Step 2: Make the frontend verify script a single deterministic gate**

Add to `frontend-ng/package.json`:

```json
"verify": "npm run audit:capabilities && npm run test:contracts && npm run test:ci && npm run build && npm run verify:build"
```

Keep `test:ci` as the Angular Vitest/coverage command; do not hide coverage output.

- [ ] **Step 3: Add a root task for the complete frontend gate**

Add to `Taskfile.yml`:

```yaml
  verify:frontend:
    summary: Verify capability inventory, architecture contracts, Angular tests, build, and fonts
    cmds:
      - cd frontend-ng && npm run verify
```

Keep `test:frontend` short enough for ordinary iteration; use `verify:frontend` before retirement completion and in CI.

- [ ] **Step 4: Run frontend verification in CI before platform builds**

After `npm ci` and binding generation in `.github/workflows/build.yml`, add:

```yaml
      - name: Verify canonical frontend
        working-directory: frontend-ng
        run: npm run verify
```

Retain the existing standalone `Typecheck frontend` CI step because the
`verify` command above does not include `npx tsc --noEmit`.

- [ ] **Step 5: Run the guardrails locally**

Run:

```bash
node --test scripts/*.test.mjs
wails3 task verify:frontend
```

Expected: PASS and a freshly generated capability matrix with no uncommitted diff.

- [ ] **Step 6: Commit the guardrails**

```bash
git add frontend-ng/package.json Taskfile.yml .github/workflows/build.yml scripts/verify-frontend-convergence.mjs scripts/verify-frontend-convergence.test.mjs docs/frontend/capability-matrix.md
git commit -m "ci(frontend): guard the canonical Angular architecture"
```

### Task 8: Final Cross-Stack Verification and Handoff

**Files:**
- Modify: `docs/frontend/capability-matrix.md` only when regeneration changes verified evidence.
- Modify: `docs/design/README.md` or `AGENTS.md` only when a completed gate changes a documented fact.
- Read: `docs/frontend/capability-matrix.md`
- Read: `docs/superpowers/specs/2026-07-25-frontend-convergence-design.md`
- Read: `AGENTS.md`

**Interfaces:**
- Produces: verified convergence result and a bounded follow-on backlog grouped by app family.
- Does not produce: app-wide visual redesign, broad CoreGO compliance rewrites, or a new transport provider.

- [ ] **Step 1: Verify the frontend from a clean generated state**

Run:

```bash
wails3 task common:generate:bindings
cd frontend-ng
npm ci
npm run verify
```

Expected: capability inventory, Node contract tests, all Angular tests, production build, and font-asset verification pass.

- [ ] **Step 2: Verify changed Go packages**

Run:

```bash
go test ./go/pkg/connection ./go/pkg/desktop ./go/pkg/server
```

Expected: PASS. Ensure no development app owns `127.0.0.1:9099` before running the desktop package.

- [ ] **Step 3: Verify repository-wide Go health**

Run one command at a time:

```bash
go work sync
go vet ./go/...
wails3 task test:go
```

Expected: PASS. If a pre-existing broad failure appears outside changed scope, record its exact command and output separately; do not change unrelated packages to manufacture a green result.

- [ ] **Step 4: Run the CoreGO audit as a no-regression comparison**

Run:

```bash
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh .
```

Compare the eight code-wrongness dimensions named in Task 4 with the pre-change baseline. Expected: none increase. Report the remaining repository backlog as non-compliant if the audit does.

- [ ] **Step 5: Verify native application behaviour**

Run:

```bash
wails3 task dev
```

Smoke:

1. Native window opens as normal host software.
2. Desktop OS mode opens its two seeded windows and router-derived menus.
3. App Shell mode renders the same selected app/route.
4. Phone and tablet modes reuse the same app state.
5. `#/w/:app` opens the selected app through the same registry.
6. `#/tray` renders the compact tray surface.
7. Native transport connects to `ws://localhost:9099/wails/ws`.
8. A browser client can load the served UI and select a `wss://`/proxy transport without a WebView-only API dependency.
9. Fonts follow the pack policy and all requested font/icon assets return non-HTML responses.

- [ ] **Step 6: Verify CLI independence**

After closing the development GUI, run:

```bash
go run ./go/cmd/lthn version
go run ./go/cmd/lthn help
go run ./go/cmd/lthn ai models
```

Start `lthn serve` only long enough to probe `/v1/health`, then stop it cleanly. Expected: these paths do not construct a Wails window or require Angular health.

Use this bounded probe:

```bash
go run ./go/cmd/lthn serve --port 18080 >/dev/null 2>&1 &
serve_pid=$!
trap 'kill "$serve_pid" 2>/dev/null || true' EXIT
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if curl --fail --silent http://127.0.0.1:18080/v1/health; then
    break
  fi
  sleep 1
done
curl --fail --silent http://127.0.0.1:18080/v1/health
kill -TERM "$serve_pid"
wait "$serve_pid" || true
trap - EXIT
```

- [ ] **Step 7: Verify final Git state**

Run:

```bash
git status --short --branch
git diff --check
git log --oneline --decorate -10
```

Expected: `main` contains the reviewable convergence commits, no untracked diagnostic files remain, and the branch is ahead of `origin/main` only by intentional local commits.

- [ ] **Step 8: Write the follow-on app-family order from evidence**

Use `docs/frontend/capability-matrix.md` to create separate reviewed plans in this order:

```text
1. Control + Chat + Models + Telemetry
2. Files + Terminal + Repositories + Tasks
3. Plugin View + Marketplace + OpenCode
4. Operations + Observe
5. Planning + Office + Sales + Marketing
6. ML Lab
7. Settings, shell ergonomics, accessibility, and cross-device polish
```

Within each family: promote runtime-proven paths to `live`, make supported subsets `partial`, repair `dormant` contracts, label intentional fixtures `design-only`, then improve the design-to-code fidelity. Keep each family independently testable and do not restore retired frontend copies.

- [ ] **Step 9: Commit any verification-only documentation update**

If the matrix or recovery documentation changed from verified facts:

```bash
git add docs/frontend/capability-matrix.md docs/design/README.md AGENTS.md
git commit -m "docs(frontend): record convergence verification"
```

If verification produced no document diff, do not create an empty commit.
