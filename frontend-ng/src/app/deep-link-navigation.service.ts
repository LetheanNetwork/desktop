import { DestroyRef, InjectionToken, Service, inject } from '@angular/core';
import { Router } from '@angular/router';
import { Events } from '@wailsio/runtime';
import { APPS } from './desktop/desktop.data';
import { readDesktopRouteCatalog, routeSegmentsForWindow } from './desktop/desktop-route-tree';

const NAVIGATE_EVENT = 'navigate';
const TRAY_OPEN_EVENT = 'lthn:tray:open';
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
    const offNavigate = this.events.on(NAVIGATE_EVENT, (payload) => {
      void this.navigate(payload);
    });
    const offTray = this.events.on(TRAY_OPEN_EVENT, (payload) => {
      void this.navigateTrayTarget(payload);
    });
    this.destroyRef.onDestroy(() => {
      offNavigate();
      offTray();
    });
  }

  private async navigate(payload: unknown): Promise<void> {
    const target = asDeepLinkTarget(payload);
    if (!target) return;

    const appId = appForDeepLink(target);
    if (!appId) return;

    await this.navigateToApp(appId);
  }

  private async navigateTrayTarget(payload: unknown): Promise<void> {
    if (typeof payload !== 'string' || this.router.url === '/tray') return;
    const target = payload.trim().toLocaleLowerCase('en-GB');
    if (target === 'desktop') {
      await this.router.navigateByUrl('/');
      return;
    }

    const mapped: Record<string, readonly [app: string, sub?: string]> = {
      chat: ['chat'],
      models: ['control', 'models'],
      settings: ['settings'],
      telemetry: ['telemetry'],
      tools: ['marketplace'],
    };
    const destination = target.startsWith('plugin:') ? ['marketplace'] : mapped[target];
    if (!destination) return;
    await this.navigateToApp(destination[0], destination[1]);
  }

  private async navigateToApp(appId: string, sub?: string): Promise<void> {
    const segments = routeSegmentsForWindow(this.routeCatalog, appId, sub);
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
