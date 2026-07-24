import { DesktopComponent } from './desktop/desktop.component';
import { routes } from './app.routes';
import { StandaloneAppHost } from './standalone-app-host';
import { DESKTOP_APP_ROUTES, readDesktopRouteCatalog } from './desktop/desktop-route-tree';
import { CATEGORIES, CTRL_NAV } from './desktop/desktop.data';

describe('app routes', () => {
  it('keeps the desktop shell and native-window host routes lazy', async () => {
    expect(routes[0]).toMatchObject({
      path: '',
      loadComponent: expect.any(Function),
      children: DESKTOP_APP_ROUTES,
    });
    expect(routes[1]).toMatchObject({
      path: 'w/:app',
      loadComponent: expect.any(Function),
    });
    await expect(routes[0].loadComponent!()).resolves.toBe(DesktopComponent);
    await expect(routes[1].loadComponent!()).resolves.toBe(StandaloneAppHost);
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
});
