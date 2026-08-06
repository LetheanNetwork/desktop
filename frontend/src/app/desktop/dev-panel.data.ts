export type DevPanelRoute =
  | 'control-panel'
  | 'explorer'
  | 'search'
  | 'git'
  | 'build'
  | 'process'
  | 'containers'
  | 'repos'
  | 'devops'
  | 'tenant'
  | 'forge'
  | 'marketplace'
  | 'tasks'
  | 'terminal';

export type DevPanelKind = 'table' | 'tree' | 'console' | 'term' | 'cards' | 'kanban';
export type DevPanelColumnType = 'status' | 'mono' | 'num';

export interface DevPanelColumn {
  readonly key: string;
  readonly label: string;
  readonly type?: DevPanelColumnType;
}

export type DevPanelTableRow = Readonly<Record<string, string | number>>;
export type DevPanelTile = readonly [value: string, label: string];
export type DevPanelTreeNode = readonly [depth: number, name: string, folder: 0 | 1];
export type DevPanelConsoleLine = readonly [time: string, source: string, message: string];

export interface DevPanelCard {
  readonly title: string;
  readonly sub: string;
  readonly icon: string;
}

export interface DevPanelKanbanColumn {
  readonly name: string;
  readonly cards: readonly string[];
}

export interface DevPanelTableFixture {
  readonly kind: 'table';
  readonly tiles?: readonly DevPanelTile[];
  readonly cols: string;
  readonly rows: string;
}

export interface DevPanelTreeFixture {
  readonly kind: 'tree';
  readonly nodes: readonly DevPanelTreeNode[];
}

export interface DevPanelConsoleFixture {
  readonly kind: 'console';
  readonly lines: readonly DevPanelConsoleLine[];
}

export interface DevPanelTerminalFixture {
  readonly kind: 'term';
  readonly lines: readonly string[];
}

export interface DevPanelCardsFixture {
  readonly kind: 'cards';
  readonly cards: readonly DevPanelCard[];
}

export interface DevPanelKanbanFixture {
  readonly kind: 'kanban';
  readonly columns: readonly DevPanelKanbanColumn[];
}

export type DevPanelFixture =
  | DevPanelTableFixture
  | DevPanelTreeFixture
  | DevPanelConsoleFixture
  | DevPanelTerminalFixture
  | DevPanelCardsFixture
  | DevPanelKanbanFixture;

/**
 * Angular's template consumes one optional-field view while the fixture
 * catalogue below is checked against the stricter discriminated union.
 */
export interface DevPanelView {
  readonly kind: DevPanelKind | 'empty';
  readonly tiles?: readonly DevPanelTile[];
  readonly cols?: string;
  readonly rows?: string;
  readonly nodes?: readonly DevPanelTreeNode[];
  readonly lines?: readonly DevPanelConsoleLine[] | readonly string[];
  readonly cards?: readonly DevPanelCard[];
  readonly columns?: readonly DevPanelKanbanColumn[];
}

const serialiseColumns = (columns: readonly DevPanelColumn[]): string => JSON.stringify(columns);
const serialiseRows = (rows: readonly DevPanelTableRow[]): string => JSON.stringify(rows);

export const DEV_PANEL_CATALOGUE = {
  'control-panel': {
    kind: 'table',
    tiles: [
      ['9/9', $localize`:Developer metric@@devPanel.metric.services:Services`],
      ['34%', $localize`:Developer metric@@devPanel.metric.cpu:CPU`],
      ['18.4 GB', $localize`:Developer metric@@devPanel.metric.memory:Mem`],
      ['6d 4h', $localize`:Developer metric@@devPanel.metric.uptime:Uptime`],
    ],
    cols: serialiseColumns([
      { key: 'svc', label: $localize`:Developer table column@@devPanel.column.service:Service` },
      {
        key: 'st',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
      {
        key: 'port',
        label: $localize`:Developer table column@@devPanel.column.port:Port`,
        type: 'mono',
      },
    ]),
    rows: serialiseRows([
      { svc: 'inference', st: 'running', port: '1988' },
      { svc: 'api', st: 'running', port: '8080' },
      { svc: 'lethernet', st: 'active', port: '4666' },
      { svc: 'store', st: 'idle', port: '5432' },
    ]),
  },
  explorer: {
    kind: 'tree',
    nodes: [
      [0, 'src', 1],
      [1, 'app', 1],
      [2, 'app.ts', 0],
      [2, 'app.routes.ts', 0],
      [1, 'elements', 1],
      [1, 'frame', 1],
      [0, 'go', 1],
      [1, 'cmd', 1],
      [1, 'pkg', 1],
      [0, 'README.md', 0],
      [0, 'go.work', 0],
    ],
  },
  search: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'file',
        label: $localize`:Developer table column@@devPanel.column.file:File`,
        type: 'mono',
      },
      {
        key: 'ln',
        label: $localize`:Developer table column@@devPanel.column.line:Ln`,
        type: 'num',
      },
      { key: 'match', label: $localize`:Developer table column@@devPanel.column.match:Match` },
    ]),
    rows: serialiseRows([
      { file: 'app.routes.ts', ln: 58, match: 'loadComponent build' },
      { file: 'build.component.ts', ln: 12, match: 'class BuildComponent' },
      { file: 'process.go', ln: 204, match: 'func (s *Service) Start' },
    ]),
  },
  git: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'change',
        label: $localize`:Developer table column@@devPanel.column.change:Change`,
      },
      {
        key: 'file',
        label: $localize`:Developer table column@@devPanel.column.file:File`,
        type: 'mono',
      },
    ]),
    rows: serialiseRows([
      { change: 'Modified', file: 'app.routes.ts' },
      { change: 'Added', file: 'desktop.component.ts' },
      { change: 'Deleted', file: 'legacy/old.ts' },
    ]),
  },
  build: {
    kind: 'console',
    lines: [
      ['09:20:01', 'go', 'build ./cmd/lthn …'],
      ['09:20:07', 'go', 'ok — 3 targets · 4.1s'],
      ['09:20:07', 'ng', 'build frontend …'],
      ['09:20:24', 'ng', '✓ 25 lazy chunks · 1.2 MB'],
    ],
  },
  process: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'pid',
        label: $localize`:Developer table column@@devPanel.column.pid:PID`,
        type: 'mono',
      },
      {
        key: 'cmd',
        label: $localize`:Developer table column@@devPanel.column.command:Command`,
      },
      {
        key: 'status',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
      {
        key: 'cpu',
        label: $localize`:Developer table column@@devPanel.column.cpuPercent:CPU %`,
        type: 'num',
      },
    ]),
    rows: serialiseRows([
      { pid: '4821', cmd: 'lthn-inference serve', status: 'running', cpu: 142 },
      { pid: '5201', cmd: 'go-pool worker', status: 'active', cpu: 3.2 },
      { pid: '5410', cmd: 'go-proxy stratum', status: 'running', cpu: 1.1 },
    ]),
  },
  containers: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'name',
        label: $localize`:Developer table column@@devPanel.column.container:Container`,
      },
      {
        key: 'image',
        label: $localize`:Developer table column@@devPanel.column.image:Image`,
        type: 'mono',
      },
      {
        key: 'st',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
      {
        key: 'ports',
        label: $localize`:Developer table column@@devPanel.column.ports:Ports`,
        type: 'mono',
      },
    ]),
    rows: serialiseRows([
      { name: 'forgejo', image: 'codeberg/forgejo', st: 'running', ports: '3000' },
      { name: 'n8n', image: 'n8nio/n8n', st: 'running', ports: '5678' },
      { name: 'vaultwarden', image: 'vaultwarden/server', st: 'active', ports: '8000' },
      { name: 'sonarqube', image: 'sonarqube:lts', st: 'idle', ports: '9000' },
    ]),
  },
  repos: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'repo',
        label: $localize`:Developer table column@@devPanel.column.repository:Repository`,
      },
      {
        key: 'branch',
        label: $localize`:Developer table column@@devPanel.column.branch:Branch`,
        type: 'mono',
      },
      {
        key: 'st',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
      {
        key: 'sync',
        label: $localize`:Developer table column@@devPanel.column.sync:Sync`,
        type: 'mono',
      },
    ]),
    rows: serialiseRows([
      { repo: 'core/go', branch: 'dev', st: 'running', sync: '↑ 0 ↓ 0' },
      { repo: 'core/ide', branch: 'dev', st: 'active', sync: '↑ 2 ↓ 0' },
      { repo: 'core/play', branch: 'dev', st: 'idle', sync: '↑ 0 ↓ 1' },
      { repo: 'desktop', branch: 'main', st: 'running', sync: '↑ 0 ↓ 0' },
    ]),
  },
  devops: {
    kind: 'table',
    cols: serialiseColumns([
      { key: 'play', label: $localize`:Developer table column@@devPanel.column.play:Play` },
      {
        key: 'host',
        label: $localize`:Developer table column@@devPanel.column.host:Host`,
        type: 'mono',
      },
      {
        key: 'st',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
      {
        key: 'dur',
        label: $localize`:Developer table column@@devPanel.column.duration:Dur`,
        type: 'mono',
      },
    ]),
    rows: serialiseRows([
      { play: 'provision', host: 'vi-01.lan', st: 'running', dur: '12s' },
      { play: 'deploy inference', host: 'vi-02.lan', st: 'active', dur: '41s' },
      { play: 'harden', host: 'hoplite-7', st: 'idle', dur: '—' },
    ]),
  },
  tenant: {
    kind: 'table',
    cols: serialiseColumns([
      {
        key: 'tenant',
        label: $localize`:Developer table column@@devPanel.column.tenant:Tenant`,
      },
      { key: 'plan', label: $localize`:Developer table column@@devPanel.column.plan:Plan` },
      {
        key: 'users',
        label: $localize`:Developer table column@@devPanel.column.users:Users`,
        type: 'num',
      },
      {
        key: 'st',
        label: $localize`:Developer table column@@devPanel.column.state:State`,
        type: 'status',
      },
    ]),
    rows: serialiseRows([
      { tenant: 'lethean', plan: 'Sovereign', users: 42, st: 'running' },
      { tenant: 'host-uk', plan: 'Business', users: 18, st: 'active' },
      { tenant: 'studio', plan: 'Starter', users: 3, st: 'idle' },
    ]),
  },
  forge: {
    kind: 'cards',
    cards: [
      { title: 'go-service', sub: 'Go microservice', icon: 'server' },
      { title: 'angular-app', sub: 'Angular 18 SPA', icon: 'window-maximize' },
      { title: 'stim-bundle', sub: 'CorePlay bundle', icon: 'gamepad' },
      { title: 'mcp-server', sub: 'MCP tool server', icon: 'plug' },
    ],
  },
  marketplace: {
    kind: 'cards',
    cards: [
      { title: 'Vi', sub: 'AI copilot', icon: 'robot' },
      { title: 'Prettier', sub: 'Formatter', icon: 'wand-magic-sparkles' },
      { title: 'GitLens', sub: 'Git insight', icon: 'code-branch' },
      { title: 'Docker', sub: 'Containers', icon: 'cubes-stacked' },
    ],
  },
  tasks: {
    kind: 'kanban',
    columns: [
      { name: 'To do', cards: ['Rip out legacy UI', 'Wire process panel', 'LSP in editor'] },
      { name: 'In progress', cards: ['Desktop shell', 'i18n viewer'] },
      { name: 'Done', cards: ['Command palette', 'Theme editor', 'App categories'] },
    ],
  },
  terminal: {
    kind: 'term',
    lines: [
      'lthn@desktop ~/core % core build',
      '→ 3 targets built in 4.1s',
      'lthn@desktop ~/core % core play mega-lo-mania',
      '→ launching STIM bundle (retroarch)…',
      'lthn@desktop ~/core % ',
    ],
  },
} satisfies Readonly<Record<DevPanelRoute, DevPanelFixture>>;

export const EMPTY_DEV_PANEL: DevPanelView = { kind: 'empty' };

export function devPanelFor(route: string): DevPanelView {
  return DEV_PANEL_CATALOGUE[route as DevPanelRoute] ?? EMPTY_DEV_PANEL;
}

/**
 * The honest empty state for a developer route, or null when the panel always
 * has rows. A clean working tree is a result, not an absent panel, so Git says
 * so rather than rendering a table with nothing in it.
 */
export function devPanelEmptyFor(route: string): [string, string, string] | null {
  return route === 'git'
    ? [
        $localize`:Source control empty-state title@@devPanel.git.noChanges:No changes`,
        'code-branch',
        $localize`:Source control empty-state message@@devPanel.git.cleanTree:Working tree clean — nothing to commit`,
      ]
    : null;
}
