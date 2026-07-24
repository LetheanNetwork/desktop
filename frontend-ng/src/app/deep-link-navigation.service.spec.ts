import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router, RouterOutlet, Routes } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import {
  DEEP_LINK_EVENTS,
  DeepLinkEventSource,
  DeepLinkNavigationService,
} from './deep-link-navigation.service';

@Component({
  imports: [RouterOutlet],
  template: '<router-outlet />',
})
class TestShell {}

@Component({ template: '' })
class TestSurface {}

const testRoutes: Routes = [
  {
    path: '',
    component: TestShell,
    children: [
      {
        path: 'ai',
        data: {
          category: 'ai',
          title: 'AI',
          icon: 'robot',
          kind: 'category',
        },
        children: [
          {
            path: 'chat',
            loadComponent: () => Promise.resolve(TestSurface),
            data: {
              category: 'ai',
              title: 'Chat',
              icon: 'comments',
              kind: 'app',
              app: 'chat',
            },
          },
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
        children: [
          {
            path: 'flows',
            loadComponent: () => Promise.resolve(TestSurface),
            data: {
              category: 'agents',
              title: 'Flows',
              icon: 'diagram-project',
              kind: 'app',
              app: 'surface-agents-flows',
            },
          },
        ],
      },
    ],
  },
];

describe('DeepLinkNavigationService', () => {
  const listeners = new Map<string, (payload: unknown) => void>();
  const events: DeepLinkEventSource = {
    on: vi.fn((name, handler) => {
      listeners.set(name, handler);
      return () => listeners.delete(name);
    }),
  };
  let harness: RouterTestingHarness;
  let router: Router;

  beforeEach(async () => {
    listeners.clear();
    TestBed.configureTestingModule({
      providers: [provideRouter(testRoutes), { provide: DEEP_LINK_EVENTS, useValue: events }],
    });
    harness = await RouterTestingHarness.create();
    router = TestBed.inject(Router);
    TestBed.inject(DeepLinkNavigationService);
  });

  afterEach(() => TestBed.resetTestingModule());

  const navigate = async (payload: unknown): Promise<void> => {
    listeners.get('navigate')?.(payload);
    await harness.fixture.whenStable();
  };

  it('routes a registered app id through the desktop route catalogue', async () => {
    expect(events.on).toHaveBeenCalledWith('navigate', expect.any(Function));

    await navigate({ action: 'chat', resource: '', id: '' });

    expect(router.url).toBe('/ai/chat');
  });

  it('maps the MCP directory target to the real tooling catalogue surface', async () => {
    await navigate({ action: 'mcp', resource: 'directory', id: '' });

    expect(router.url).toBe('/agents/flows');
  });

  it('leaves the current route unchanged for unknown or malformed targets', async () => {
    await navigate({ action: 'chat', resource: 'unexpected', id: '' });
    expect(router.url).toBe('/');

    await navigate({ action: 'unknown', resource: '', id: '' });
    expect(router.url).toBe('/');

    await navigate(null);
    expect(router.url).toBe('/');
  });
});
