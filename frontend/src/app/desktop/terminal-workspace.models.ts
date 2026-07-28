// SPDX-License-Identifier: EUPL-1.2

const DOCUMENT_VERSION = 1;
const MAXIMUM_TABS = 32;
const MAXIMUM_IDENTIFIER_BYTES = 128;
const MAXIMUM_TITLE_BYTES = 256;
const MAXIMUM_PATH_BYTES = 512;
const IDENTIFIER = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/u;
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/u;
const WINDOWS_ABSOLUTE = /^(?:[A-Za-z]:[\\/]|\\\\)/u;

export interface TerminalWorkspaceRef {
  readonly mountId: string;
  readonly path: string;
  readonly repository: string;
}

export interface TerminalWorkspaceTab {
  readonly key: string;
  readonly title: string;
  readonly kind: 'shell' | 'agent';
  readonly workspace: TerminalWorkspaceRef;
  readonly sharedAgentId: string;
}

export interface TerminalWorkspace {
  readonly activeKey: string;
  readonly tabs: readonly TerminalWorkspaceTab[];
}

export interface TerminalWorkspaceSnapshot {
  readonly version: number;
  readonly revision: number;
  readonly updatedAt: string;
  readonly workspace: TerminalWorkspace;
}

export interface RuntimeTerminalTab {
  readonly key: string;
  readonly title: string;
  readonly repo?: string;
  readonly cwd?: string;
  readonly mountId?: string;
  readonly workspacePath?: string;
  readonly attachId?: string;
  readonly sharedAgentId?: string;
  readonly command?: readonly string[];
  readonly shared?: boolean;
  readonly exited?: boolean;
}

export interface RestoredTerminalTab extends RuntimeTerminalTab {
  readonly kind: 'shell' | 'agent';
}

export function parseTerminalWorkspaceSnapshot(raw: unknown): TerminalWorkspaceSnapshot {
  const record = exactRecord(raw, ['version', 'revision', 'updatedAt', 'workspace']);
  const version = requiredInteger(record['version'], 1, DOCUMENT_VERSION);
  if (version !== DOCUMENT_VERSION) throw invalidTerminalWorkspace();
  const revision = requiredInteger(record['revision'], 0, Number.MAX_SAFE_INTEGER);
  const updatedAt = optionalString(record['updatedAt'], 64);
  if ((revision === 0 && updatedAt !== '') || (revision > 0 && !validTimestamp(updatedAt))) {
    throw invalidTerminalWorkspace();
  }
  return {
    version,
    revision,
    updatedAt,
    workspace: parseTerminalWorkspace(record['workspace']),
  };
}

export function parseTerminalWorkspace(raw: unknown): TerminalWorkspace {
  const record = exactRecord(raw, ['activeKey', 'tabs']);
  const activeKey = optionalIdentifier(record['activeKey']);
  if (!Array.isArray(record['tabs']) || record['tabs'].length > MAXIMUM_TABS) {
    throw invalidTerminalWorkspace();
  }

  const tabs = record['tabs'].map(parseTerminalWorkspaceTab);
  const keys = new Set<string>();
  for (const tab of tabs) {
    if (keys.has(tab.key)) throw invalidTerminalWorkspace();
    keys.add(tab.key);
  }
  if (activeKey && !keys.has(activeKey)) throw invalidTerminalWorkspace();
  return { activeKey, tabs };
}

export function terminalWorkspaceFromTabs(
  runtimeTabs: readonly RuntimeTerminalTab[],
  requestedActiveKey: string,
): TerminalWorkspace {
  const tabs: TerminalWorkspaceTab[] = [];
  const seen = new Set<string>();

  for (const runtime of runtimeTabs.slice(0, MAXIMUM_TABS)) {
    if (!validIdentifier(runtime.key) || seen.has(runtime.key)) continue;
    const title = safeTitle(runtime.title);
    if (!title) continue;

    if (runtime.shared) {
      const sharedAgentId = runtime.sharedAgentId || runtime.attachId || '';
      if (!validIdentifier(sharedAgentId)) continue;
      tabs.push({
        key: runtime.key,
        title,
        kind: 'agent',
        workspace: emptyWorkspaceRef(),
        sharedAgentId,
      });
      seen.add(runtime.key);
      continue;
    }

    tabs.push({
      key: runtime.key,
      title,
      kind: 'shell',
      workspace: runtimeWorkspaceRef(runtime),
      sharedAgentId: '',
    });
    seen.add(runtime.key);
  }

  const activeKey = seen.has(requestedActiveKey) ? requestedActiveKey : (tabs[0]?.key ?? '');
  return { activeKey, tabs };
}

export function terminalTabsFromWorkspace(
  value: TerminalWorkspace,
  liveAgentIDs: ReadonlySet<string>,
): readonly RestoredTerminalTab[] {
  const workspace = parseTerminalWorkspace(value);
  return workspace.tabs.map((tab) => {
    if (tab.kind === 'agent') {
      const live = liveAgentIDs.has(tab.sharedAgentId);
      return {
        key: tab.key,
        title: tab.title,
        kind: 'agent',
        shared: true,
        sharedAgentId: tab.sharedAgentId,
        attachId: live ? tab.sharedAgentId : undefined,
        exited: !live,
      };
    }
    return {
      key: tab.key,
      title: tab.title,
      kind: 'shell',
      ...(tab.workspace.repository ? { repo: tab.workspace.repository } : {}),
      ...(tab.workspace.mountId
        ? {
            mountId: tab.workspace.mountId,
            workspacePath: tab.workspace.path,
          }
        : {}),
      exited: false,
    };
  });
}

function parseTerminalWorkspaceTab(raw: unknown): TerminalWorkspaceTab {
  const record = exactRecord(raw, ['key', 'title', 'kind', 'workspace', 'sharedAgentId']);
  const kind = record['kind'];
  if (kind !== 'shell' && kind !== 'agent') throw invalidTerminalWorkspace();
  const sharedAgentId = optionalIdentifier(record['sharedAgentId']);
  if ((kind === 'agent') !== Boolean(sharedAgentId)) throw invalidTerminalWorkspace();
  return {
    key: requiredIdentifier(record['key']),
    title: requiredString(record['title'], MAXIMUM_TITLE_BYTES),
    kind,
    workspace: parseWorkspaceRef(record['workspace']),
    sharedAgentId,
  };
}

function parseWorkspaceRef(raw: unknown): TerminalWorkspaceRef {
  const record = exactRecord(raw, ['mountId', 'path', 'repository']);
  const mountId = optionalIdentifier(record['mountId']);
  const repository = optionalIdentifier(record['repository']);
  const path = optionalString(record['path'], MAXIMUM_PATH_BYTES);
  if ((mountId && repository) || (path && (!mountId || !validProviderPath(path)))) {
    throw invalidTerminalWorkspace();
  }
  return { mountId, path, repository };
}

function runtimeWorkspaceRef(tab: RuntimeTerminalTab): TerminalWorkspaceRef {
  if (validIdentifier(tab.repo ?? '')) {
    return { mountId: '', path: '', repository: tab.repo ?? '' };
  }
  if (validIdentifier(tab.mountId ?? '')) {
    const path = tab.workspacePath ?? '';
    if (!path || validProviderPath(path)) {
      return { mountId: tab.mountId ?? '', path, repository: '' };
    }
  }
  return emptyWorkspaceRef();
}

function emptyWorkspaceRef(): TerminalWorkspaceRef {
  return { mountId: '', path: '', repository: '' };
}

function exactRecord(value: unknown, allowed: readonly string[]): Record<string, unknown> {
  if (
    !value ||
    typeof value !== 'object' ||
    Array.isArray(value) ||
    Object.keys(value).some((key) => !allowed.includes(key))
  ) {
    throw invalidTerminalWorkspace();
  }
  return value as Record<string, unknown>;
}

function requiredIdentifier(value: unknown): string {
  if (typeof value !== 'string' || !validIdentifier(value)) throw invalidTerminalWorkspace();
  return value;
}

function optionalIdentifier(value: unknown): string {
  if (value === undefined || value === '') return '';
  return requiredIdentifier(value);
}

function validIdentifier(value: string): boolean {
  return (
    value.length <= MAXIMUM_IDENTIFIER_BYTES &&
    new TextEncoder().encode(value).length <= MAXIMUM_IDENTIFIER_BYTES &&
    IDENTIFIER.test(value)
  );
}

function requiredString(value: unknown, maximumBytes: number): string {
  if (
    typeof value !== 'string' ||
    !value ||
    new TextEncoder().encode(value).length > maximumBytes ||
    CONTROL_CHARACTERS.test(value)
  ) {
    throw invalidTerminalWorkspace();
  }
  return value;
}

function optionalString(value: unknown, maximumBytes: number): string {
  if (value === undefined || value === '') return '';
  return requiredString(value, maximumBytes);
}

function safeTitle(value: string): string {
  if (!value || CONTROL_CHARACTERS.test(value)) return '';
  const bytes = new TextEncoder().encode(value);
  if (bytes.length <= MAXIMUM_TITLE_BYTES) return value;
  let title = value;
  while (title && new TextEncoder().encode(title).length > MAXIMUM_TITLE_BYTES) {
    title = title.slice(0, -1);
  }
  return title;
}

function requiredInteger(value: unknown, minimum: number, maximum: number): number {
  if (!Number.isSafeInteger(value) || (value as number) < minimum || (value as number) > maximum) {
    throw invalidTerminalWorkspace();
  }
  return value as number;
}

function validProviderPath(value: string): boolean {
  if (
    !value ||
    value.length > MAXIMUM_PATH_BYTES ||
    value.startsWith('/') ||
    WINDOWS_ABSOLUTE.test(value) ||
    value.includes('\\') ||
    CONTROL_CHARACTERS.test(value)
  ) {
    return false;
  }
  const parts = value.split('/');
  return parts.every((part) => part !== '' && part !== '.' && part !== '..');
}

function validTimestamp(value: string): boolean {
  return Boolean(value && Number.isFinite(Date.parse(value)));
}

function invalidTerminalWorkspace(): Error {
  return new Error('The desktop returned an invalid terminal workspace.');
}
