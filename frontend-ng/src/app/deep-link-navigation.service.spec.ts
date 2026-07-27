import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router, RouterOutlet, Routes } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { DeepLinkNavigationService } from './deep-link-navigation.service';

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
  let harness: RouterTestingHarness;
  let router: Router;
  let service: DeepLinkNavigationService;

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [provideRouter(testRoutes)],
    });
    harness = await RouterTestingHarness.create();
    router = TestBed.inject(Router);
    service = TestBed.inject(DeepLinkNavigationService);
  });

  afterEach(() => TestBed.resetTestingModule());

  const navigate = async (payload: unknown): Promise<void> => {
    await service.navigateDeepLink(payload);
    await harness.fixture.whenStable();
  };

  const trayOpen = async (payload: unknown): Promise<void> => {
    await service.navigateTrayTarget(payload);
    await harness.fixture.whenStable();
  };

  it('routes a registered app id through the desktop route catalogue', async () => {
    await navigate({ action: 'chat', resource: '', id: '' });

    expect(router.url).toBe('/ai/chat');
  });

  it('maps the MCP directory target to the real tooling catalogue surface', async () => {
    await navigate({ action: 'mcp', resource: 'directory', id: '' });

    expect(router.url).toBe('/agents/flows');
  });

  it('routes tray launch events through the same desktop catalogue', async () => {
    await trayOpen('chat');

    expect(router.url).toBe('/ai/chat');
  });

  it('leaves the current route unchanged for unknown or malformed targets', async () => {
    await navigate({ action: 'chat', resource: 'unexpected', id: '' });
    expect(router.url).toBe('/');

    await navigate({ action: 'unknown', resource: '', id: '' });
    expect(router.url).toBe('/');

    await navigate(null);
    expect(router.url).toBe('/');
  });

  it('rejects extra keys and oversized compatibility payloads', async () => {
    await navigate({ action: 'chat', resource: '', id: '', path: '/Users/private' });
    expect(router.url).toBe('/');

    await navigate({ action: 'x'.repeat(65), resource: '', id: '' });
    expect(router.url).toBe('/');
  });
});
