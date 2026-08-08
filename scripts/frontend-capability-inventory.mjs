import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

import { gitLines, pathExists } from './repository.mjs';

const COMPOSITE_COMPONENT = 'frontend/src/app/desktop/apps/composite.app.ts';
const DEV_PANEL_COMPONENT = 'frontend/src/app/desktop/apps/dev-panel.app.ts';

// The component each application route loads. A panelled application loads the
// shared composite shell; the pane rows below carry the panes' own components.
const BASE_COMPONENTS = Object.freeze({
  welcome: 'frontend/src/app/desktop/apps/welcome.app.ts',
  control: 'frontend/src/app/desktop/apps/control.app.ts',
  telemetry: 'frontend/src/app/desktop/apps/telemetry.app.ts',
  activity: COMPOSITE_COMPONENT,
  operations: COMPOSITE_COMPONENT,
  marketplace: COMPOSITE_COMPONENT,
  settings: 'frontend/src/app/desktop/apps/settings.app.ts',
  ide: COMPOSITE_COMPONENT,
  scm: COMPOSITE_COMPONENT,
  databases: COMPOSITE_COMPONENT,
  'project-manager': COMPOSITE_COMPONENT,
  mail: 'frontend/src/app/desktop/surfaces/office/mail.ts',
  documents: 'frontend/src/app/desktop/surfaces/office/documents.ts',
  crm: COMPOSITE_COMPONENT,
  marketing: COMPOSITE_COMPONENT,
  tenant: DEV_PANEL_COMPONENT,
  chat: 'frontend/src/app/desktop/apps/chat.app.ts',
  agents: COMPOSITE_COMPONENT,
  'ml-lab': COMPOSITE_COMPONENT,
  opencode: COMPOSITE_COMPONENT,
  games: 'frontend/src/app/desktop/apps/games.app.ts',
  files: COMPOSITE_COMPONENT,
  notepad: 'frontend/src/app/desktop/apps/notepad.app.ts',
  lethernet: 'frontend/src/app/desktop/apps/lethernet.app.ts',
});

// One hash route per application. Pane routes are these plus the pane path.
const BASE_ROUTES = Object.freeze({
  welcome: '/system/welcome',
  control: '/system/control',
  telemetry: '/system/telemetry',
  activity: '/system/activity',
  operations: '/system/operations',
  marketplace: '/system/marketplace',
  settings: '/system/settings',
  ide: '/developer/ide',
  scm: '/developer/scm',
  databases: '/developer/databases',
  'project-manager': '/office/project-manager',
  mail: '/office/mail',
  documents: '/office/documents',
  crm: '/office/crm',
  marketing: '/office/marketing',
  tenant: '/office/tenant',
  chat: '/ai/chat',
  agents: '/ai/agents',
  'ml-lab': '/ai/ml-lab',
  opencode: '/ai/opencode',
  games: '/media/games',
  files: '/tools/files',
  notepad: '/tools/notepad',
  lethernet: '/networking/lethernet',
});

const SPECIALISED_EVIDENCE = Object.freeze({
  chat: ['frontend/src/app/desktop/desktop-ai.service.ts'],
  control: [
    'frontend/src/app/desktop/desktop-live-data.service.ts',
    'frontend/src/app/desktop/desktop-services-bridge.service.ts',
  ],
  telemetry: ['frontend/src/app/desktop/desktop-system-monitor-bridge.service.ts'],
  'ide.terminal': ['frontend/src/app/desktop/desktop-live-data.service.ts'],
  files: ['frontend/src/app/desktop/desktop-files-bridge.service.ts'],
  settings: ['frontend/src/app/desktop/preferences.service.ts'],
  'surface-agents-terminal': ['frontend/src/app/desktop/surfaces/agents/terminal-session.ts'],
  'surface-extensions-marketplace': [
    'frontend/src/app/desktop/surfaces/extensions/marketplace.ts',
  ],
  'surface-extensions-plugin-view': [
    'frontend/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts',
  ],
  'surface-extensions-opencode-shim': [
    'frontend/src/app/desktop/surfaces/extensions/opencode-shim.ts',
  ],
  'surface-opencode-sessions': ['frontend/src/app/desktop/surfaces/opencode/sessions.ts'],
  'surface-opencode-profiles': ['frontend/src/app/desktop/surfaces/opencode/profiles.ts'],
  'surface-office-files': [
    'frontend/src/app/desktop/apps/files.app.ts',
    'frontend/src/app/desktop/desktop-files-bridge.service.ts',
  ],
});

const CONTRACT_PATTERN = /(?:bridgeMethod|loadEndpoint|endpoint):\s*['"`]([^'"`]+)['"`]/g;

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
 *   paneApps: CapabilityEntry[];
 *   entries: CapabilityEntry[];
 * }} CapabilityReport
 */

export async function collectCapabilityEvidence(repoRoot) {
  const baseApps = await readBaseApps(repoRoot);
  const paneApps = await readPaneApps(repoRoot);
  const entries = [...baseApps, ...paneApps];

  for (const entry of entries) {
    const hasIntegration = entry.contracts.length > 0 || entry.specialisedEvidence.length > 0;
    const allContractsResolve = entry.resolvedContracts === entry.contracts.length;
    entry.sourceState = !hasIntegration
      ? 'design-fixture'
      : allContractsResolve && entry.evidence.length > 0
        ? 'integrated'
        : 'unresolved';
    entry.limitations.push('Runtime path not certified by this source audit.');
  }
  return { baseApps, paneApps, entries };
}

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

async function readBaseApps(repoRoot) {
  const source = await readFile(
    join(repoRoot, 'frontend/src/app/desktop/desktop-catalogue.data.ts'),
    'utf8',
  );
  const block = source.slice(
    source.indexOf('export const APPS:'),
    source.indexOf('export const ORDER:'),
  );
  const ids = [...block.matchAll(/^\s{2}'?([a-z][a-z0-9-]*)'?:\s*\{/gm)].map((match) => match[1]);
  for (const id of ids) {
    if (!BASE_ROUTES[id] || !BASE_COMPONENTS[id]) {
      throw new Error(`Application "${id}" has no audited route or component`);
    }
  }
  return Promise.all(
    ids.map((id) => entryFromComponent(repoRoot, id, BASE_ROUTES[id], BASE_COMPONENTS[id])),
  );
}

/**
 * Every pane declared in desktop-panes.data.ts, at its real deep-link route.
 * A surface pane is audited under its surface id so its specialised evidence
 * still resolves; a developer or application-view pane is audited under
 * `<app>.<pane>`.
 */
async function readPaneApps(repoRoot) {
  const surfaces = await readSurfaceComponents(repoRoot);
  const source = await readFile(
    join(repoRoot, 'frontend/src/app/desktop/desktop-panes.data.ts'),
    'utf8',
  );
  const block = source.slice(source.indexOf('export const APP_PANES:'), source.indexOf('];'));
  const panes = [
    ...block.matchAll(/\{\s*app: '([^']+)',\s*path: '([^']+)',\s*(surface|view|dev): '([^']+)',/g),
  ];
  return Promise.all(
    panes.map(([, app, path, kind, value]) => {
      const route = `${BASE_ROUTES[app]}/${path}`;
      if (kind === 'surface') {
        const component = surfaces.get(value);
        if (!component) throw new Error(`Pane ${app}/${path} names unknown surface "${value}"`);
        return entryFromComponent(repoRoot, value, route, component);
      }
      const component =
        kind === 'dev'
          ? DEV_PANEL_COMPONENT
          : `frontend/src/app/desktop/apps/${value}.app.ts`;
      return entryFromComponent(repoRoot, `${app}.${path}`, route, component);
    }),
  );
}

async function readSurfaceComponents(repoRoot) {
  const source = await readFile(
    join(repoRoot, 'frontend/src/app/desktop/surfaces/surface-registry.ts'),
    'utf8',
  );
  const block = source.slice(
    source.indexOf('export const SURFACE_DEFINITIONS:'),
    source.indexOf('export function surfaceAppId'),
  );
  return new Map(
    [...block.matchAll(/group: '([^']+)', route: '([^']+)'/g)].map(([, group, route]) => [
      `surface-${group}-${route}`,
      `frontend/src/app/desktop/surfaces/${group}/${route}.ts`,
    ]),
  );
}

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
    evidence: [...resolutions.flatMap(({ evidence }) => evidence), ...presentSpecialisedEvidence],
    limitations: [],
    resolvedContracts: resolutions.filter(({ resolved }) => resolved).length,
    specialisedEvidence,
    sourceState: 'unresolved',
  };
}

async function resolveContract(repoRoot, contract) {
  const goFiles = (await gitLines(repoRoot, ['ls-files', 'go'])).filter(
    (path) => path.endsWith('.go') && !path.endsWith('_test.go'),
  );
  const method = contract.match(
    /^dappco\.re\/lthn\/desktop\/(pkg\/.+)\.([A-Z][A-Za-z0-9_]*)\.([A-Za-z][A-Za-z0-9_]*)$/,
  );
  if (method) {
    const [, packagePath, receiverType, methodName] = method;
    const candidates = goFiles.filter((path) => path.startsWith(`go/${packagePath}/`));
    const declaration = new RegExp(
      `func\\s*\\([^)]*\\*${receiverType}\\)\\s*${methodName}\\s*\\(`,
    );
    const evidence = [];
    for (const path of candidates) {
      const source = await readFile(join(repoRoot, path), 'utf8');
      if (declaration.test(source)) evidence.push(`${path}#${receiverType}.${methodName}`);
    }
    return { resolved: evidence.length > 0, evidence };
  }

  if (contract.startsWith('/')) {
    // A load endpoint may carry query parameters the Go route
    // declaration never states — resolve on the path alone.
    const route = contract.split('?')[0];
    const evidence = [];
    for (const path of goFiles) {
      const source = await readFile(join(repoRoot, path), 'utf8');
      if (source.includes(route)) evidence.push(`${path}#${route}`);
    }
    return { resolved: evidence.length > 0, evidence };
  }

  return { resolved: false, evidence: [] };
}

const modulePath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(modulePath), '..');
const isMain = process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

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
