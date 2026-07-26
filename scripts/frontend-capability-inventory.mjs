import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

import { gitLines, pathExists } from './repository.mjs';

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
  files: ['frontend-ng/src/app/desktop/desktop-files-bridge.service.ts'],
  settings: ['frontend-ng/src/app/desktop/preferences.service.ts'],
  'surface-agents-terminal': ['frontend-ng/src/app/desktop/surfaces/agents/terminal-session.ts'],
  'surface-extensions-marketplace': [
    'frontend-ng/src/app/desktop/surfaces/extensions/marketplace.ts',
  ],
  'surface-extensions-plugin-view': [
    'frontend-ng/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts',
  ],
  'surface-extensions-opencode-shim': [
    'frontend-ng/src/app/desktop/surfaces/extensions/opencode-shim.ts',
  ],
  'surface-office-files': [
    'frontend-ng/src/app/desktop/apps/files.app.ts',
    'frontend-ng/src/app/desktop/desktop-files-bridge.service.ts',
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
 *   surfaceApps: CapabilityEntry[];
 *   entries: CapabilityEntry[];
 * }} CapabilityReport
 */

export async function collectCapabilityEvidence(repoRoot) {
  const baseApps = await readBaseApps(repoRoot);
  const surfaceApps = await readSurfaceApps(repoRoot);
  const entries = [...baseApps, ...surfaceApps];

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
  return { baseApps, surfaceApps, entries };
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
    join(repoRoot, 'frontend-ng/src/app/desktop/desktop-catalogue.data.ts'),
    'utf8',
  );
  const block = source.slice(
    source.indexOf('export const APPS:'),
    source.indexOf('export const CATEGORIES:'),
  );
  const ids = [...block.matchAll(/^\s{2}([a-z][a-z0-9_-]+):\s*\{/gm)].map((match) => match[1]);
  return Promise.all(
    ids.map((id) => entryFromComponent(repoRoot, id, BASE_ROUTES[id], BASE_COMPONENTS[id])),
  );
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
  const definitions = [
    ...block.matchAll(/group:\s*'([^']+)'[\s\S]*?route:\s*'([^']+)'[\s\S]*?title:/g),
  ];
  return Promise.all(
    definitions.map(([, group, route]) => {
      const id = `surface-${group}-${route}`;
      return entryFromComponent(
        repoRoot,
        id,
        `/${group}/${route}`,
        `frontend-ng/src/app/desktop/surfaces/${group}/${route}.ts`,
      );
    }),
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
    /^dappco\.re\/lthn\/desktop\/(pkg\/.+)\.Service\.([A-Za-z][A-Za-z0-9_]*)$/,
  );
  if (method) {
    const [, packagePath, methodName] = method;
    const candidates = goFiles.filter((path) => path.startsWith(`go/${packagePath}/`));
    const declaration = new RegExp(`func\\s*\\([^)]*\\*Service\\)\\s*${methodName}\\s*\\(`);
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
