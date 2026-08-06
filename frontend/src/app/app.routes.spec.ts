import { DesktopComponent } from './desktop/desktop.component';
import { routes } from './app.routes';
import { StandaloneAppHost } from './standalone-app-host';
import { TrayPanel } from './tray-panel/tray-panel';
import { DESKTOP_APP_ROUTES, readDesktopRouteCatalog } from './desktop/desktop-route-tree';
import { CATEGORIES, CTRL_NAV } from './desktop/desktop.data';
import { APP_PANES } from './desktop/desktop-panes.data';

describe('app routes', () => {
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
      path: 'w/:app/:pane',
      loadComponent: expect.any(Function),
    });
    expect(routes[3]).toMatchObject({
      path: 'tray',
      loadComponent: expect.any(Function),
    });
    await expect(routes[0].loadComponent!()).resolves.toBe(DesktopComponent);
    await expect(routes[1].loadComponent!()).resolves.toBe(StandaloneAppHost);
    await expect(routes[2].loadComponent!()).resolves.toBe(StandaloneAppHost);
    await expect(routes[3].loadComponent!()).resolves.toBe(TrayPanel);
  });

  it('builds category, app, and sub-view metadata from desktop data', () => {
    const catalog = readDesktopRouteCatalog(routes);

    expect(catalog.categories.map(({ id }) => id)).toEqual(CATEGORIES.map(({ id }) => id));
    expect(catalog.apps['control']).toMatchObject({
      category: 'system',
      title: 'Control',
      icon: 'cube',
      defaultSub: 'models',
    });
    expect(catalog.apps['control'].children).toEqual(CTRL_NAV);
    expect(catalog.apps['control'].loadComponent).toEqual(expect.any(Function));
  });

  // Where every ported surface ended up when sixty-six launchable applications
  // were folded down to twenty-three. The surfaces did not change — their route
  // did, from a top-level /<group>/<path> of their own to a pane of the
  // application that owns them.
  const surfaceRoutes: Record<string, string> = {
    'surface-agents-activity': '/ai/agents/activity',
    'surface-agents-code': '/ai/agents/code',
    'surface-agents-connect': '/ai/agents/connect',
    'surface-agents-dispatch': '/ai/agents/dispatch',
    'surface-agents-flows': '/ai/agents/flows',
    'surface-agents-scan': '/ai/agents/scan',
    'surface-agents-tasks': '/ai/agents/tasks',
    'surface-agents-terminal': '/ai/agents/terminal',
    'surface-agents-workspaces': '/ai/agents/workspaces',
    'surface-coding-deploys': '/developer/scm/deploys',
    'surface-coding-issues': '/office/project-manager/issues',
    'surface-coding-prs': '/developer/scm/pull-requests',
    'surface-coding-repos': '/developer/scm/repositories',
    'surface-marketing-analytics': '/office/marketing/analytics',
    'surface-marketing-audience': '/office/marketing/audience',
    'surface-marketing-campaigns': '/office/marketing/campaigns',
    'surface-marketing-content': '/office/marketing/content',
    'surface-marketing-social': '/office/marketing/social',
    'surface-ml-lab-duckdb': '/developer/databases/duckdb',
    'surface-ml-lab-influx': '/developer/databases/influxdb',
    'surface-ml-lab-lora': '/ai/ml-lab/lora',
    'surface-ml-lab-ml-lab': '/ai/ml-lab/workbench',
    'surface-ml-lab-models': '/ai/ml-lab/models',
    'surface-observe-activity': '/system/activity/observe',
    'surface-office-files': '/tools/files/workspace',
    'surface-operations-incidents': '/system/operations/incidents',
    'surface-operations-runbooks': '/system/operations/runbooks',
    'surface-operations-status': '/system/operations/status',
    'surface-planning-backlog': '/office/project-manager/backlog',
    'surface-planning-calendar': '/office/project-manager/calendar',
    'surface-planning-retros': '/office/project-manager/retros',
    'surface-planning-roadmap': '/office/project-manager/roadmap',
    'surface-planning-sprints': '/office/project-manager/sprints',
    'surface-planning-today': '/office/project-manager/today',
    'surface-sales-contacts': '/office/crm/contacts',
    'surface-sales-deals': '/office/crm/deals',
    'surface-sales-forecast': '/office/crm/forecast',
    'surface-sales-pipeline': '/office/crm/pipeline',
    'surface-extensions-marketplace': '/system/marketplace/store',
    'surface-extensions-plugin-view': '/system/marketplace/plugin-view',
    'surface-extensions-opencode-shim': '/system/marketplace/opencode',
  };

  it('exposes every ported surface as a deep-linkable pane of its application', () => {
    const catalog = readDesktopRouteCatalog(routes);
    const routed = APP_PANES.flatMap(({ app, path, surface }) => {
      if (!surface) return [];
      const menuApp = catalog.apps[app];
      expect(menuApp, `no route for application ${app}`).toBeDefined();
      expect(
        menuApp.children.some(([childPath]) => childPath === path),
        `${app} publishes no ${path} pane`,
      ).toBe(true);
      return [[surface, `/${menuApp.categoryPath}/${menuApp.path}/${path}`] as const];
    });

    expect(Object.fromEntries(routed)).toEqual(surfaceRoutes);
  });

  it('promotes Mail and Documents to applications of their own', () => {
    const catalog = readDesktopRouteCatalog(routes);

    expect(catalog.apps['mail']).toMatchObject({ categoryPath: 'office', path: 'mail' });
    expect(catalog.apps['documents']).toMatchObject({
      categoryPath: 'office',
      path: 'documents',
    });
    expect(catalog.apps['mail'].children).toEqual([]);
    expect(catalog.apps['documents'].children).toEqual([]);
  });

  it('retires every surface, developer-panel, and duplicate application id', () => {
    const catalog = readDesktopRouteCatalog(routes);
    const retired = [
      'cpanel',
      'explorer',
      'codesearch',
      'terminal',
      'build',
      'procmon',
      'containers',
      'repos',
      'forge',
      'devops',
      'tasks',
      'surface-agents-flows',
      'surface-office-mail',
      'surface-planning-today',
    ];

    expect(retired.filter((id) => catalog.apps[id])).toEqual([]);
    expect(Object.keys(catalog.apps)).toHaveLength(23);
  });
});
