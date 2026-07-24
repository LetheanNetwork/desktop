import { DestroyRef, InjectionToken, Service, inject } from '@angular/core';
import { Router } from '@angular/router';
import { Events } from '@wailsio/runtime';
import { APPS } from './desktop/desktop.data';
import { readDesktopRouteCatalog, routeSegmentsForWindow } from './desktop/desktop-route-tree';

const NAVIGATE_EVENT = 'navigate';
const MCP_DIRECTORY_APP_ID = 'surface-agents-flows';

export interface DeepLinkEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const DEEP_LINK_EVENTS = new InjectionToken<DeepLinkEventSource>('DEEP_LINK_EVENTS', {
  providedIn: 'root',
  factory: () => ({
    on(name, handler): () => void {
      return Events.On(name, (event) => handler(event.data));
    },
  }),
});

interface DeepLinkTarget {
  readonly action: string;
  readonly resource: string;
  readonly id: string;
}

@Service()
export class DeepLinkNavigationService {
  private readonly router = inject(Router);
  private readonly events = inject(DEEP_LINK_EVENTS);
  private readonly destroyRef = inject(DestroyRef);
  private readonly routeCatalog = readDesktopRouteCatalog(this.router.config);

  constructor() {
    const off = this.events.on(NAVIGATE_EVENT, (payload) => {
      void this.navigate(payload);
    });
    this.destroyRef.onDestroy(off);
  }

  private async navigate(payload: unknown): Promise<void> {
    const target = asDeepLinkTarget(payload);
    if (!target) return;

    const appId = appForDeepLink(target);
    if (!appId) return;

    const segments = routeSegmentsForWindow(this.routeCatalog, appId);
    if (!segments.length) return;

    await this.router.navigateByUrl(`/${segments.join('/')}`);
  }
}

function asDeepLinkTarget(payload: unknown): DeepLinkTarget | null {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return null;
  }

  const record = payload as Record<string, unknown>;
  if (
    typeof record['action'] !== 'string' ||
    typeof record['resource'] !== 'string' ||
    typeof record['id'] !== 'string'
  ) {
    return null;
  }

  return {
    action: record['action'].trim().toLocaleLowerCase('en-GB'),
    resource: record['resource'].trim().toLocaleLowerCase('en-GB'),
    id: record['id'].trim(),
  };
}

function appForDeepLink(target: DeepLinkTarget): string | null {
  if (target.action === 'mcp' && target.resource === 'directory' && target.id === '') {
    return MCP_DIRECTORY_APP_ID;
  }
  if (target.resource === '' && target.id === '' && Object.hasOwn(APPS, target.action)) {
    return target.action;
  }
  return null;
}
