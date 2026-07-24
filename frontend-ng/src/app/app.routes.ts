import { Routes } from '@angular/router';
import { DESKTOP_APP_ROUTES } from './desktop/desktop-route-tree';

export const routes: Routes = [
  {
    path: '',
    loadComponent: () =>
      import('./desktop/desktop.component').then(({ DesktopComponent }) => DesktopComponent),
    children: DESKTOP_APP_ROUTES,
  },
  {
    path: 'w/:app',
    loadComponent: () =>
      import('./standalone-app-host').then(({ StandaloneAppHost }) => StandaloneAppHost),
  },
];
