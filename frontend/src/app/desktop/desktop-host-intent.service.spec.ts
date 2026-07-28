import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router, RouterOutlet, Routes } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DESKTOP_HOST_EVENTS,
  DesktopHostEventSource,
  DesktopHostIntentService,
} from './desktop-host-intent.service';

@Component({
  imports: [RouterOutlet],
  template: '<router-outlet />',
})
class TestShell {}

@Component({ template: '' })
class TestSurface {}

const appRoute = (
  path: string,
  app: string,
  title: string,
  children: Routes = [],
) => ({
  path,
  loadComponent: () => Promise.resolve(TestSurface),
  data: {
    category: path,
    title,
    icon: 'square',
    kind: 'app',
    app,
  },
  children,
});

const subRoute = (path: string, app: string) => ({
  path,
  component: TestSurface,
  data: {
    category: app,
    title: path,
    icon: 'square',
    kind: 'subview',
    app,
    sub: path,
  },
});

const testRoutes: Routes = [
  {
    path: '',
    component: TestShell,
    children: [
      {
        path: 'office',
        data: {
          category: 'office',
          title: 'Office',
          icon: 'briefcase',
          kind: 'category',
        },
        children: [appRoute('files', 'files', 'Files')],
      },
      {
        path: 'system',
        data: {
          category: 'system',
          title: 'System',
          icon: 'gear',
          kind: 'category',
        },
        children: [
          appRoute('settings', 'settings', 'Settings'),
          appRoute('control', 'control', 'Control', [subRoute('models', 'control')]),
        ],
      },
      {
        path: 'agents',
        data: {
          category: 'agents',
          title: 'Agents',
          icon: 'robot',
          kind: 'category',
        },
        children: [appRoute('terminal', 'terminal', 'Terminal')],
      },
      {
        path: 'ai',
        data: {
          category: 'ai',
          title: 'AI',
          icon: 'robot',
          kind: 'category',
        },
        children: [appRoute('chat', 'chat', 'Chat')],
      },
    ],
  },
];

describe('DesktopHostIntentService', () => {
  let listeners: Map<string, (payload: unknown) => void>;
  let events: DesktopHostEventSource;
  let harness: RouterTestingHarness;
  let router: Router;
  let service: DesktopHostIntentService;

  beforeEach(async () => {
    listeners = new Map();
    events = {
      on: vi.fn((name, handler) => {
        listeners.set(name, handler);
        return () => listeners.delete(name);
      }),
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter(testRoutes),
        { provide: DESKTOP_HOST_EVENTS, useValue: events },
        {
          provide: ConnectionManagerService,
          useValue: { offline: signal(false) },
        },
      ],
    });
    harness = await RouterTestingHarness.create();
    router = TestBed.inject(Router);
    service = TestBed.inject(DesktopHostIntentService);
  });

  afterEach(() => TestBed.resetTestingModule());

  const emit = async (name: string, payload: unknown): Promise<void> => {
    listeners.get(name)?.(payload);
    await harness.fixture.whenStable();
  };

  it('registers one native event boundary plus compatibility navigation listeners', () => {
    expect(events.on).toHaveBeenCalledWith('lthn:host:intent', expect.any(Function));
    expect(events.on).toHaveBeenCalledWith('navigate', expect.any(Function));
    expect(events.on).toHaveBeenCalledWith('lthn:tray:open', expect.any(Function));
    expect(events.on).toHaveBeenCalledTimes(6);
  });

  it('routes an ordinary opaque item to Files and lets the lazy surface claim it', async () => {
    await emit('lthn:host:intent', {
      kind: 'open-items',
      items: [
        {
          mountId: 'documents',
          path: 'Notes/today.txt',
          name: 'today.txt',
          kind: 'file',
          mediaType: 'text/plain',
        },
      ],
      navigation: { app: 'files', action: 'open' },
    });

    expect(router.url).toBe('/office/files');
    expect(service.claimItems('files')).toEqual([
      {
        mountId: 'documents',
        path: 'Notes/today.txt',
        name: 'today.txt',
        kind: 'file',
        mediaType: 'text/plain',
      },
    ]);
    expect(service.claimItems('files')).toBeNull();
  });

  it('delivers later native items to an already-mounted Files surface', async () => {
    const received: unknown[] = [];
    const off = service.onItems('files', (items) => received.push(items));

    await emit('lthn:host:intent', {
      kind: 'open-items',
      items: [
        {
          mountId: 'documents',
          path: 'later.txt',
          name: 'later.txt',
          kind: 'file',
          mediaType: 'text/plain',
        },
      ],
      navigation: { app: 'files', action: 'open' },
    });

    expect(received).toHaveLength(1);
    expect(service.claimItems('files')).toBeNull();
    off();
  });

  it('routes one opaque .lthn item to Settings for explicit import review', async () => {
    await emit('lthn:host:intent', {
      kind: 'open-items',
      items: [
        {
          mountId: 'host-profile',
          path: 'profile.lthn',
          name: 'profile.lthn',
          kind: 'file',
          mediaType: 'application/x-lethean',
        },
      ],
      navigation: { app: 'settings', action: 'import' },
    });

    expect(router.url).toBe('/system/settings');
    expect(service.claimItems('settings')?.[0]?.name).toBe('profile.lthn');
  });

  it('ignores malformed, oversized, raw-path, and unauthorised item intents', async () => {
    const invalid = [
      null,
      { kind: 'unknown' },
      {
        kind: 'open-items',
        items: [
          {
            mountId: 'documents',
            path: '/Users/private/notes.txt',
            name: 'notes.txt',
            kind: 'file',
            mediaType: 'text/plain',
          },
        ],
        navigation: { app: 'files', action: 'open' },
      },
      {
        kind: 'open-items',
        items: [
          {
            mountId: 'documents',
            path: 'notes.txt',
            name: 'x'.repeat(256),
            kind: 'file',
            mediaType: 'text/plain',
          },
        ],
        navigation: { app: 'files', action: 'open' },
      },
      {
        kind: 'open-items',
        items: [
          {
            mountId: 'documents',
            path: 'notes.txt',
            name: 'notes.txt',
            kind: 'file',
            mediaType: 'text/plain',
          },
        ],
        navigation: { app: 'unknown', action: 'open' },
      },
      {
        kind: 'open-items',
        items: [
          {
            mountId: 'documents',
            path: 'notes.txt',
            name: 'notes.txt',
            kind: 'file',
            mediaType: 'text/plain',
          },
        ],
        navigation: { app: 'files', action: 'execute' },
      },
    ];

    for (const payload of invalid) {
      await emit('lthn:host:intent', payload);
    }

    expect(router.url).toBe('/');
    expect(service.claimItems('files')).toBeNull();
    expect(service.claimItems('settings')).toBeNull();
  });

  it('routes only known notification intents and bounded action ids', async () => {
    await emit('lthn:host:intent', {
      kind: 'notification',
      notification: {
        id: 'native-123',
        intentId: 'model-runtime',
        event: 'action',
        actionId: 'open',
      },
    });
    expect(router.url).toBe('/system/control/models');
    expect(service.lastNotification()?.intentId).toBe('model-runtime');

    await router.navigateByUrl('/');
    await emit('lthn:host:intent', {
      kind: 'notification',
      notification: {
        id: 'native-456',
        intentId: 'unknown',
        event: 'click',
      },
    });
    await emit('lthn:host:intent', {
      kind: 'notification',
      notification: {
        id: 'native-789',
        intentId: 'managed-services',
        event: 'action',
        actionId: 'run-command',
      },
    });
    expect(router.url).toBe('/');
  });

  it('keeps application policy separate from verified host permission state', async () => {
    await emit('lthn:host:intent', {
      kind: 'permission',
      permission: {
        id: 'notifications',
        policy: 'deny',
        host: 'granted',
      },
    });

    expect(service.permission('notifications')).toEqual({
      id: 'notifications',
      policy: 'deny',
      host: 'granted',
    });

    await emit('lthn:host:intent', {
      kind: 'permission',
      permission: {
        id: 'arbitrary-host-capability',
        policy: 'allow',
        host: 'granted',
      },
    });
    expect(service.permission('arbitrary-host-capability')).toBeNull();
  });

  it('validates compatibility deep-link and tray events through the same router', async () => {
    await emit('navigate', { action: 'chat', resource: '', id: '' });
    expect(router.url).toBe('/ai/chat');

    await emit('lthn:tray:open', 'models');
    expect(router.url).toBe('/system/control/models');
  });

  it('does not register any Wails event listener in offline demo mode', async () => {
    TestBed.resetTestingModule();
    const offlineEvents: DesktopHostEventSource = { on: vi.fn(() => () => undefined) };
    TestBed.configureTestingModule({
      providers: [
        provideRouter(testRoutes),
        { provide: DESKTOP_HOST_EVENTS, useValue: offlineEvents },
        {
          provide: ConnectionManagerService,
          useValue: { offline: signal(true) },
        },
      ],
    });
    await RouterTestingHarness.create();

    TestBed.inject(DesktopHostIntentService);

    expect(offlineEvents.on).not.toHaveBeenCalled();
  });
});
