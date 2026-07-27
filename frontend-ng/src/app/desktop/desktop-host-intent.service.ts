import { DestroyRef, InjectionToken, Service, inject, signal } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { ConnectionManagerService } from '../connection-manager.service';
import { DeepLinkNavigationService } from '../deep-link-navigation.service';
import { validMountId, validRelativePath } from './apps/files/files-view-state';

const HOST_INTENT_EVENT = 'lthn:host:intent';
const MAX_HOST_INTENT_BYTES = 32 * 1024;
const MAX_HOST_ITEMS = 16;
const MAX_ITEM_NAME_BYTES = 255;
const MAX_MEDIA_TYPE_BYTES = 127;
const MAX_NOTIFICATION_ID_BYTES = 128;
const MAX_TARGET_ID_BYTES = 64;
const MAX_HOST_COORDINATE = 32_768;
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/;
const SAFE_ID = /^[A-Za-z0-9][A-Za-z0-9._:-]*$/;

const FILE_ERROR_CODES = new Set([
  'files.invalid_input',
  'files.invalid_mount',
  'files.boundary_rejected',
  'files.capability_denied',
  'files.missing_entry',
  'files.conflict',
  'files.provider_unavailable',
  'files.limit_exceeded',
  'files.unsupported_entry',
  'files.partial_move',
]);

const PERMISSION_IDS = [
  'microphone',
  'camera',
  'geolocation',
  'notifications',
  'clipboard-read',
] as const;
const PERMISSION_POLICIES = ['default', 'allow', 'deny'] as const;
const HOST_PERMISSION_STATES = [
  'granted',
  'denied',
  'prompt',
  'restricted',
  'unsupported',
  'unknown',
] as const;

const NOTIFICATION_ROUTES = {
  'managed-services': { app: 'control', actions: ['open'] },
  'model-runtime': { app: 'control', sub: 'models', actions: ['open'] },
  'terminal-session': { app: 'terminal', actions: ['open'] },
  'files-operation': { app: 'files', actions: ['open'] },
  'desktop-update': { app: 'settings', actions: ['open'] },
} as const;

export interface DesktopHostEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const DESKTOP_HOST_EVENTS = new InjectionToken<DesktopHostEventSource>(
  'DESKTOP_HOST_EVENTS',
  {
    providedIn: 'root',
    factory: () => ({
      on(name, handler): () => void {
        return Events.On(name, (event) => handler(event.data));
      },
    }),
  },
);

export interface DesktopHostItem {
  readonly mountId: string;
  readonly path: string;
  readonly name: string;
  readonly kind: 'file' | 'directory';
  readonly mediaType: string;
}

export type DesktopPermissionID = (typeof PERMISSION_IDS)[number];
export type DesktopPermissionPolicy = (typeof PERMISSION_POLICIES)[number];
export type DesktopHostPermissionState = (typeof HOST_PERMISSION_STATES)[number];

export interface DesktopPermissionSnapshot {
  readonly id: DesktopPermissionID;
  readonly policy: DesktopPermissionPolicy;
  readonly host: DesktopHostPermissionState;
}

export interface DesktopNotificationIntent {
  readonly id: string;
  readonly intentId: keyof typeof NOTIFICATION_ROUTES;
  readonly event: 'click' | 'action' | 'dismiss';
  readonly actionId?: string;
}

interface ItemIntent {
  readonly kind: 'open-items' | 'drop-items';
  readonly items: readonly DesktopHostItem[];
  readonly navigation: {
    readonly app: 'files' | 'settings';
    readonly action: 'open' | 'import';
  };
}

/**
 * Validates every native renderer event before it can affect Angular state or
 * routing. Lazy product surfaces claim opaque Files capabilities from here;
 * native paths, commands, URLs, and arbitrary permission names are rejected.
 */
@Service()
export class DesktopHostIntentService {
  private readonly connection = inject(ConnectionManagerService);
  private readonly events = inject(DESKTOP_HOST_EVENTS);
  private readonly navigation = inject(DeepLinkNavigationService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly pendingItems = new Map<'files' | 'settings', readonly DesktopHostItem[]>();
  private readonly itemConsumers = new Map<
    'files' | 'settings',
    Set<(items: readonly DesktopHostItem[]) => void>
  >();
  private readonly permissionSnapshots = signal(
    new Map<DesktopPermissionID, DesktopPermissionSnapshot>(),
  );
  private readonly _lastNotification = signal<DesktopNotificationIntent | null>(null);
  private readonly _lastErrorCode = signal('');

  readonly lastNotification = this._lastNotification.asReadonly();
  readonly lastErrorCode = this._lastErrorCode.asReadonly();

  constructor() {
    if (this.connection.offline()) return;

    const remove = [
      this.events.on(HOST_INTENT_EVENT, (payload) => void this.dispatch(payload)),
      this.events.on('navigate', (payload) => void this.navigation.navigateDeepLink(payload)),
      this.events.on('lthn:tray:open', (payload) =>
        void this.navigation.navigateTrayTarget(payload),
      ),
      this.events.on('lthn:notification:click', (payload) =>
        void this.dispatchCompatibilityNotification('click', payload),
      ),
      this.events.on('lthn:notification:action', (payload) =>
        void this.dispatchCompatibilityNotification('action', payload),
      ),
      this.events.on('lthn:notification:dismiss', (payload) =>
        void this.dispatchCompatibilityNotification('dismiss', payload),
      ),
    ];
    this.destroyRef.onDestroy(() => remove.forEach((off) => off()));
  }

  claimItems(app: 'files' | 'settings'): readonly DesktopHostItem[] | null {
    const items = this.pendingItems.get(app) ?? null;
    this.pendingItems.delete(app);
    return items;
  }

  onItems(
    app: 'files' | 'settings',
    consumer: (items: readonly DesktopHostItem[]) => void,
  ): () => void {
    const consumers = this.itemConsumers.get(app) ?? new Set();
    consumers.add(consumer);
    this.itemConsumers.set(app, consumers);
    const pending = this.claimItems(app);
    if (pending) consumer(pending);
    return () => {
      consumers.delete(consumer);
      if (consumers.size === 0) this.itemConsumers.delete(app);
    };
  }

  permission(id: string): DesktopPermissionSnapshot | null {
    if (!isOneOf(id, PERMISSION_IDS)) return null;
    return this.permissionSnapshots().get(id) ?? null;
  }

  private async dispatch(payload: unknown): Promise<void> {
    if (!withinPayloadLimit(payload) || !isRecord(payload) || typeof payload['kind'] !== 'string') {
      return;
    }

    switch (payload['kind']) {
      case 'open-items':
      case 'drop-items': {
        const intent = asItemIntent(payload);
        if (!intent) return;
        const consumers = this.itemConsumers.get(intent.navigation.app);
        if (consumers?.size) {
          consumers.forEach((consumer) => consumer(intent.items));
        } else {
          this.pendingItems.set(intent.navigation.app, intent.items);
        }
        await this.navigation.navigateToApp(intent.navigation.app);
        return;
      }
      case 'items-unavailable': {
        if (
          hasExactKeys(payload, ['kind', 'errorCode']) &&
          typeof payload['errorCode'] === 'string' &&
          FILE_ERROR_CODES.has(payload['errorCode'])
        ) {
          this._lastErrorCode.set(payload['errorCode']);
        }
        return;
      }
      case 'notification': {
        if (!hasExactKeys(payload, ['kind', 'notification'])) return;
        const notification = asNotificationIntent(payload['notification']);
        if (!notification) return;
        await this.handleNotification(notification);
        return;
      }
      case 'permission': {
        if (!hasExactKeys(payload, ['kind', 'permission'])) return;
        const permission = asPermissionSnapshot(payload['permission']);
        if (!permission) return;
        this.permissionSnapshots.update((current) => {
          const next = new Map(current);
          next.set(permission.id, permission);
          return next;
        });
        return;
      }
    }
  }

  private async handleNotification(notification: DesktopNotificationIntent): Promise<void> {
    this._lastNotification.set(notification);
    if (notification.event === 'dismiss') return;
    const target = NOTIFICATION_ROUTES[notification.intentId];
    await this.navigation.navigateToApp(target.app, 'sub' in target ? target.sub : undefined);
  }

  private async dispatchCompatibilityNotification(
    event: DesktopNotificationIntent['event'],
    payload: unknown,
  ): Promise<void> {
    let id: unknown = payload;
    let actionId: unknown;
    if (event === 'action') {
      if (!isRecord(payload) || !hasExactKeys(payload, ['notification_id', 'action_id'])) return;
      id = payload['notification_id'];
      actionId = payload['action_id'];
    }
    if (typeof id !== 'string') return;
    const intentId = intentIDFromNativeNotification(id);
    if (!intentId) return;
    const candidate = asNotificationIntent({
      id,
      intentId,
      event,
      ...(actionId === undefined ? {} : { actionId }),
    });
    if (candidate) await this.handleNotification(candidate);
  }
}

function asItemIntent(record: Record<string, unknown>): ItemIntent | null {
  if (!hasOnlyKeys(record, ['kind', 'items', 'navigation', 'target'])) return null;
  if (!Array.isArray(record['items']) || record['items'].length === 0) return null;
  if (record['items'].length > MAX_HOST_ITEMS) return null;
  const items = record['items'].map(asHostItem);
  if (items.some((item) => item === null)) return null;

  const navigation = asItemNavigation(record['navigation']);
  if (!navigation) return null;
  const validItems = items as DesktopHostItem[];
  const isLetheanConfig =
    validItems.length === 1 &&
    validItems[0].kind === 'file' &&
    validItems[0].name.toLocaleLowerCase('en-GB').endsWith('.lthn');
  if (
    (navigation.app === 'settings' && (!isLetheanConfig || navigation.action !== 'import')) ||
    (navigation.app === 'files' && (isLetheanConfig || navigation.action !== 'open'))
  ) {
    return null;
  }
  if (record['target'] !== undefined && !validDropTarget(record['target'])) return null;

  return {
    kind: record['kind'] as ItemIntent['kind'],
    items: validItems,
    navigation,
  };
}

function asHostItem(value: unknown): DesktopHostItem | null {
  if (
    !isRecord(value) ||
    !hasExactKeys(value, ['mountId', 'path', 'name', 'kind', 'mediaType']) ||
    typeof value['mountId'] !== 'string' ||
    !validMountId(value['mountId']) ||
    typeof value['path'] !== 'string' ||
    !validRelativePath(value['path']) ||
    !boundedSafeName(value['name']) ||
    (value['kind'] !== 'file' && value['kind'] !== 'directory') ||
    typeof value['mediaType'] !== 'string' ||
    value['mediaType'].length === 0 ||
    value['mediaType'].length > MAX_MEDIA_TYPE_BYTES ||
    CONTROL_CHARACTERS.test(value['mediaType'])
  ) {
    return null;
  }
  return {
    mountId: value['mountId'],
    path: value['path'],
    name: value['name'],
    kind: value['kind'],
    mediaType: value['mediaType'],
  };
}

function asItemNavigation(
  value: unknown,
): ItemIntent['navigation'] | null {
  if (
    !isRecord(value) ||
    !hasExactKeys(value, ['app', 'action']) ||
    (value['app'] !== 'files' && value['app'] !== 'settings') ||
    (value['action'] !== 'open' && value['action'] !== 'import')
  ) {
    return null;
  }
  return { app: value['app'], action: value['action'] };
}

function asNotificationIntent(value: unknown): DesktopNotificationIntent | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ['id', 'intentId', 'event', 'actionId'])) {
    return null;
  }
  if (
    !boundedID(value['id'], MAX_NOTIFICATION_ID_BYTES) ||
    typeof value['intentId'] !== 'string' ||
    !Object.hasOwn(NOTIFICATION_ROUTES, value['intentId']) ||
    (value['event'] !== 'click' &&
      value['event'] !== 'action' &&
      value['event'] !== 'dismiss')
  ) {
    return null;
  }
  const intentId = value['intentId'] as keyof typeof NOTIFICATION_ROUTES;
  if (value['event'] === 'action') {
    if (
      typeof value['actionId'] !== 'string' ||
      !NOTIFICATION_ROUTES[intentId].actions.includes(value['actionId'] as 'open')
    ) {
      return null;
    }
  } else if (value['actionId'] !== undefined) {
    return null;
  }
  return {
    id: value['id'],
    intentId,
    event: value['event'],
    ...(typeof value['actionId'] === 'string' ? { actionId: value['actionId'] } : {}),
  };
}

function asPermissionSnapshot(value: unknown): DesktopPermissionSnapshot | null {
  if (
    !isRecord(value) ||
    !hasExactKeys(value, ['id', 'policy', 'host']) ||
    !isOneOf(value['id'], PERMISSION_IDS) ||
    !isOneOf(value['policy'], PERMISSION_POLICIES) ||
    !isOneOf(value['host'], HOST_PERMISSION_STATES)
  ) {
    return null;
  }
  return { id: value['id'], policy: value['policy'], host: value['host'] };
}

function validDropTarget(value: unknown): boolean {
  return (
    isRecord(value) &&
    hasExactKeys(value, ['id', 'x', 'y']) &&
    boundedID(value['id'], MAX_TARGET_ID_BYTES) &&
    Number.isInteger(value['x']) &&
    Number.isInteger(value['y']) &&
    (value['x'] as number) >= 0 &&
    (value['x'] as number) <= MAX_HOST_COORDINATE &&
    (value['y'] as number) >= 0 &&
    (value['y'] as number) <= MAX_HOST_COORDINATE
  );
}

function intentIDFromNativeNotification(
  id: string,
): keyof typeof NOTIFICATION_ROUTES | null {
  if (!boundedID(id, MAX_NOTIFICATION_ID_BYTES)) return null;
  for (const intentId of Object.keys(NOTIFICATION_ROUTES) as (keyof typeof NOTIFICATION_ROUTES)[]) {
    if (id === `lthn.${intentId}` || id.startsWith(`lthn.${intentId}:`)) return intentId;
  }
  return null;
}

function withinPayloadLimit(value: unknown): boolean {
  try {
    return JSON.stringify(value).length <= MAX_HOST_INTENT_BYTES;
  } catch {
    return false;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function hasExactKeys(record: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(record);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function hasOnlyKeys(record: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(record).every((key) => keys.includes(key));
}

function boundedSafeName(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= MAX_ITEM_NAME_BYTES &&
    value !== '.' &&
    value !== '..' &&
    !value.includes('/') &&
    !value.includes('\\') &&
    !CONTROL_CHARACTERS.test(value)
  );
}

function boundedID(value: unknown, maximum: number): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= maximum &&
    SAFE_ID.test(value)
  );
}

function isOneOf<const T extends readonly string[]>(
  value: unknown,
  values: T,
): value is T[number] {
  return typeof value === 'string' && values.includes(value);
}
