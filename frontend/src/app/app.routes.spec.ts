import { DesktopComponent } from './desktop/desktop.component';
import { routes } from './app.routes';
import { StandaloneAppHost } from './standalone-app-host';
import { TrayPanel } from './tray-panel/tray-panel';
import { DESKTOP_APP_ROUTES, readDesktopRouteCatalog } from './desktop/desktop-route-tree';
import { CATEGORIES, CTRL_NAV } from './desktop/desktop.data';

describe('app routes', () => {
  const portedSurfaceRoutes: Record<string, string[]> = {
    agents: [
      'activity',
      'code',
      'connect',
      'dispatch',
      'flows',
      'scan',
      'tasks',
      'terminal',
      'workspaces',
    ],
    coding: ['deploys', 'issues', 'prs', 'repos'],
    marketing: ['analytics', 'audience', 'campaigns', 'content', 'social'],
    'ml-lab': ['duckdb', 'influx', 'lora', 'ml-lab', 'models'],
    observe: ['activity'],
    office: ['documents', 'files', 'mail'],
    operations: ['incidents', 'runbooks', 'status'],
    planning: ['backlog', 'calendar', 'retros', 'roadmap', 'sprints', 'today'],
    sales: ['contacts', 'deals', 'forecast', 'pipeline'],
    extensions: ['marketplace', 'plugin-view', 'opencode-shim'],
  };

  it('keeps the desktop shell, native-window host, and tray panel routes lazy', async () => {
    expect(routes[0]).toMatchObject({
      path: '',
      loadComponent: expect.any(Function),
      children: DESKTOP_APP_ROUTES,
    });
    expect(routes[1]).toMatchObject({
      path: 'w/:app',
      loadComponent: expect.any(Function),
    });
    expect(routes[2]).toMatchObject({
      path: 'tray',
      loadComponent: expect.any(Function),
    });
    await expect(routes[0].loadComponent!()).resolves.toBe(DesktopComponent);
    await expect(routes[1].loadComponent!()).resolves.toBe(StandaloneAppHost);
    await expect(routes[2].loadComponent!()).resolves.toBe(TrayPanel);
  });

  it('builds category, app, and sub-view metadata from desktop data', () => {
    const catalog = readDesktopRouteCatalog(routes);

    expect(catalog.categories.map(({ id }) => id)).toEqual(CATEGORIES.map(({ id }) => id));
    expect(catalog.apps['control']).toMatchObject({
      category: 'system',
      title: 'Control',
      icon: 'cube',
    });
    expect(catalog.apps['control'].children).toEqual(CTRL_NAV);
    expect(catalog.apps['control'].loadComponent).toEqual(expect.any(Function));
  });

  // Where each surface group ended up when the menu bar was folded from
  // sixteen categories down to seven. The surfaces did not move — their route
  // did, from /<group>/<path> to /<category>/<path> — so this map is the
  // record of that and the reason the assertion below can still be exact.
  const surfaceHome: Record<string, string> = {
    agents: 'ai',
    'ml-lab': 'ai',
    coding: 'developer',
    observe: 'system',
    operations: 'system',
    office: 'office',
    planning: 'office',
    marketing: 'office',
    sales: 'office',
    extensions: 'tools',
  };

  it('exposes every ported Lit view as a menu-derived lazy child route', () => {
    const catalog = readDesktopRouteCatalog(routes);

    for (const [groupId, expectedPaths] of Object.entries(portedSurfaceRoutes)) {
      const categoryId = surfaceHome[groupId];
      expect(categoryId, `no home recorded for surface group ${groupId}`).toBeDefined();

      const category = catalog.categories.find(({ id }) => id === categoryId);
      expect(category, `missing ${categoryId} route category`).toBeDefined();
      expect(
        category?.apps
          .filter(({ id }) => id.startsWith(`surface-${groupId}-`))
          .map(({ path }) => path)
          .sort(),
      ).toEqual([...expectedPaths].sort());
      expect(category?.apps.every(({ loadComponent }) => typeof loadComponent === 'function')).toBe(
        true,
      );
    }
  });

  it('keeps extension surfaces at their decided hash-route paths', () => {
    const catalog = readDesktopRouteCatalog(routes);

    // Extensions folded into Tools; the leaf paths are unchanged.
    expect(catalog.apps['surface-extensions-marketplace']).toMatchObject({
      categoryPath: 'tools',
      path: 'marketplace',
    });
    expect(catalog.apps['surface-extensions-plugin-view']).toMatchObject({
      categoryPath: 'tools',
      path: 'plugin-view',
    });
    expect(catalog.apps['surface-extensions-opencode-shim']).toMatchObject({
      categoryPath: 'tools',
      path: 'opencode-shim',
    });
  });
});
