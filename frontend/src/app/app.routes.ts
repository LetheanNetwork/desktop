import { Routes } from '@angular/router';
import { DESKTOP_APP_ROUTES } from './desktop/desktop-route-tree';

export const routes: Routes = [
  {
    path: '',
    loadComponent: () =>
      import('./desktop/desktop.component').then(({ DesktopComponent }) => DesktopComponent),
    children: DESKTOP_APP_ROUTES,
  },
  // Solo shell — one application, no desktop chrome. The native window IS the
  // chrome, so a torn-off application loads here; the optional pane segment
  // carries the pane the window was showing when it left the shell.
  {
    path: 'w/:app',
    loadComponent: () =>
      import('./standalone-app-host').then(({ StandaloneAppHost }) => StandaloneAppHost),
  },
  {
    path: 'w/:app/:pane',
    loadComponent: () =>
      import('./standalone-app-host').then(({ StandaloneAppHost }) => StandaloneAppHost),
  },
  {
    path: 'tray',
    title: 'Lethean Desktop',
    loadComponent: () => import('./tray-panel/tray-panel').then(({ TrayPanel }) => TrayPanel),
  },
];
