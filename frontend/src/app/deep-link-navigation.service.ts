import { Service, inject } from '@angular/core';
import { Router } from '@angular/router';
import { APPS } from './desktop/desktop-catalogue.data';
import { readDesktopRouteCatalog, routeSegmentsForWindow } from './desktop/desktop-route-tree';

/** The tooling catalogue is the Agents application's Flows pane. */
const MCP_DIRECTORY_TARGET: readonly [app: string, sub: string] = ['agents', 'flows'];
const MAX_DEEP_LINK_FIELD_BYTES = 64;
const MAX_TRAY_TARGET_BYTES = 96;

interface DeepLinkTarget {
  readonly action: string;
  readonly resource: string;
  readonly id: string;
}

@Service()
export class DeepLinkNavigationService {
  private readonly router = inject(Router);
  private readonly routeCatalog = readDesktopRouteCatalog(this.router.config);

  async navigateDeepLink(payload: unknown): Promise<boolean> {
    const target = asDeepLinkTarget(payload);
    if (!target) return false;

    const destination = appForDeepLink(target);
    if (!destination) return false;

    return await this.navigateToApp(destination[0], destination[1]);
  }

  async navigateTrayTarget(payload: unknown): Promise<boolean> {
    if (
      typeof payload !== 'string' ||
      payload.length > MAX_TRAY_TARGET_BYTES ||
      this.router.url === '/tray'
    ) {
      return false;
    }
    const target = payload.trim().toLocaleLowerCase('en-GB');
    if (target === 'desktop') {
      return await this.router.navigateByUrl('/');
    }

    const mapped: Record<string, readonly [app: string, sub?: string]> = {
      chat: ['chat'],
      models: ['control', 'models'],
      settings: ['settings'],
      telemetry: ['telemetry'],
      tools: ['marketplace', 'store'],
    };
    const destination = target.startsWith('plugin:')
      ? (['marketplace', 'plugin-view'] as const)
      : mapped[target];
    if (!destination) return false;
    return await this.navigateToApp(destination[0], destination[1]);
  }

  async navigateToApp(appId: string, sub?: string): Promise<boolean> {
    const segments = routeSegmentsForWindow(this.routeCatalog, appId, sub);
    if (!segments.length) return false;

    return await this.router.navigateByUrl(`/${segments.join('/')}`);
  }
}

function asDeepLinkTarget(payload: unknown): DeepLinkTarget | null {
  if (!isRecord(payload) || !hasExactKeys(payload, ['action', 'resource', 'id'])) {
    return null;
  }

  if (
    !boundedString(payload['action'], MAX_DEEP_LINK_FIELD_BYTES) ||
    !boundedString(payload['resource'], MAX_DEEP_LINK_FIELD_BYTES) ||
    !boundedString(payload['id'], MAX_DEEP_LINK_FIELD_BYTES)
  ) {
    return null;
  }

  return {
    action: payload['action'].trim().toLocaleLowerCase('en-GB'),
    resource: payload['resource'].trim().toLocaleLowerCase('en-GB'),
    id: payload['id'].trim(),
  };
}

function appForDeepLink(target: DeepLinkTarget): readonly [app: string, sub?: string] | null {
  if (target.action === 'mcp' && target.resource === 'directory' && target.id === '') {
    return MCP_DIRECTORY_TARGET;
  }
  if (target.resource === '' && target.id === '' && Object.hasOwn(APPS, target.action)) {
    return [target.action];
  }
  return null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function hasExactKeys(record: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(record);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function boundedString(value: unknown, maximum: number): value is string {
  return (
    typeof value === 'string' &&
    value.length <= maximum &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}
