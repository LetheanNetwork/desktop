import { Type } from '@angular/core';
import { ActivatedRouteSnapshot, Route, Routes } from '@angular/router';
import { APPS, CATEGORIES, CTRL_NAV, GAMES_NAV, SETTINGS_NAV } from './desktop.data';
import { APP_REGISTRY, AppComponentLoader, AppView } from './apps/app-view';

export type AppNavItem = readonly [path: string, icon: string, title: string];

export interface DesktopRouteData {
  category: string;
  title: string;
  icon: string;
  kind: 'category' | 'app' | 'subview';
  app?: string;
  sub?: string;
  hint?: string;
}

export interface DesktopMenuApp {
  id: string;
  path: string;
  category: string;
  categoryPath: string;
  title: string;
  icon: string;
  hint: string;
  children: AppNavItem[];
  loadComponent: AppComponentLoader;
}

export interface DesktopMenuCategory {
  id: string;
  path: string;
  title: string;
  icon: string;
  apps: DesktopMenuApp[];
}

export interface DesktopRouteCatalog {
  categories: DesktopMenuCategory[];
  apps: Record<string, DesktopMenuApp>;
}

export interface DesktopRouteTarget {
  app: string;
  sub?: string;
}

const appNav: Partial<Record<string, readonly AppNavItem[]>> = {
  control: CTRL_NAV,
  games: GAMES_NAV,
  settings: SETTINGS_NAV,
};

const dataOf = (route: Route): DesktopRouteData | null => {
  const data = route.data as Partial<DesktopRouteData> | undefined;
  return data?.category && data.title && data.icon && data.kind ? (data as DesktopRouteData) : null;
};

const appRoute = (categoryId: string, appId: string): Route => {
  const app = APPS[appId];
  const loadComponent = APP_REGISTRY[appId];
  if (!app || !loadComponent) {
    throw new Error(`No route component registered for desktop app "${appId}"`);
  }

  const nav = appNav[appId] ?? [];
  const children: Routes = nav.length
    ? [
        {
          path: '',
          pathMatch: 'full',
          redirectTo: app.defaultSub ?? nav[0][0],
        },
        ...nav.map(([path, icon, title]): Route => ({
          path,
          data: {
            category: categoryId,
            title,
            icon,
            kind: 'subview',
            app: appId,
            sub: path,
          } satisfies DesktopRouteData,
          // A componentless leaf still needs an activation target to satisfy
          // Angular's route validator. The parent app component owns the view.
          children: [],
        })),
      ]
    : [];

  return {
    path: app.route ?? app.id,
    loadComponent,
    data: {
      category: categoryId,
      title: app.title,
      icon: app.icon,
      hint: app.hint,
      kind: 'app',
      app: appId,
    } satisfies DesktopRouteData,
    children,
  };
};

/** Category/app routes nested beneath the DesktopComponent OS shell. */
export const DESKTOP_APP_ROUTES: Routes = CATEGORIES.map((category) => ({
  path: category.id,
  data: {
    category: category.id,
    title: category.label,
    icon: category.icon,
    kind: 'category',
  } satisfies DesktopRouteData,
  children: category.apps.map((appId) => appRoute(category.id, appId)),
}));

/** Read menu and app-host metadata from Router.config, never desktop.data. */
export function readDesktopRouteCatalog(config: Routes): DesktopRouteCatalog {
  const shell =
    config.find((route) => route.path === '' && route.children === DESKTOP_APP_ROUTES) ??
    config.find((route) => route.path === '' && route.children?.length);
  const categories: DesktopMenuCategory[] = [];
  const apps: Record<string, DesktopMenuApp> = {};

  for (const categoryRoute of shell?.children ?? []) {
    const categoryData = dataOf(categoryRoute);
    if (categoryData?.kind !== 'category' || !categoryRoute.path) continue;

    const menuCategory: DesktopMenuCategory = {
      id: categoryData.category,
      path: categoryRoute.path,
      title: categoryData.title,
      icon: categoryData.icon,
      apps: [],
    };

    for (const route of categoryRoute.children ?? []) {
      const appData = dataOf(route);
      if (appData?.kind !== 'app' || !appData.app || !route.path || !route.loadComponent) {
        continue;
      }

      const children: AppNavItem[] = (route.children ?? [])
        .map((child): AppNavItem | null => {
          const childData = dataOf(child);
          return childData?.kind === 'subview' && child.path
            ? [child.path, childData.icon, childData.title]
            : null;
        })
        .filter((child): child is AppNavItem => child !== null);

      const menuApp: DesktopMenuApp = {
        id: appData.app,
        path: route.path,
        category: categoryData.category,
        categoryPath: categoryRoute.path,
        title: appData.title,
        icon: appData.icon,
        hint: appData.hint ?? '',
        children,
        loadComponent: route.loadComponent as AppComponentLoader,
      };
      menuCategory.apps.push(menuApp);
      apps[menuApp.id] = menuApp;
    }
    categories.push(menuCategory);
  }

  return { categories, apps };
}

export function routeSegmentsForWindow(
  catalog: DesktopRouteCatalog,
  appId: string,
  sub?: string,
): string[] {
  const app = catalog.apps[appId];
  if (!app) return [];
  const child = app.children.find(([path]) => path === sub);
  return [app.categoryPath, app.path, ...(child ? [child[0]] : [])];
}

export function desktopTargetFromSnapshot(
  snapshot: ActivatedRouteSnapshot,
): DesktopRouteTarget | null {
  let cursor: ActivatedRouteSnapshot | null = snapshot;
  let target: DesktopRouteTarget | null = null;
  while (cursor) {
    const data = cursor.data as Partial<DesktopRouteData>;
    if (data.app) {
      target = {
        app: data.app,
        ...(data.sub ? { sub: data.sub } : {}),
      };
    }
    cursor = cursor.firstChild;
  }
  return target;
}

export function asAppViewType(component: Type<unknown>): Type<AppView> {
  return component as Type<AppView>;
}
