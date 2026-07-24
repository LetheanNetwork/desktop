import {
  ApplicationConfig,
  inject,
  isDevMode,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideEffects } from '@ngrx/effects';
import { provideRouterStore } from '@ngrx/router-store';
import { provideState, provideStore } from '@ngrx/store';
import { provideStoreDevtools } from '@ngrx/store-devtools';
import { provideRouter, withHashLocation } from '@angular/router';
import { routes } from './app.routes';
import { DesktopEffects } from './store/desktop.effects';
import { desktopFeature } from './store/desktop.reducer';
import { DesktopMcpService } from './desktop/desktop-mcp.service';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    // Hash routing: the app runs inside a wails:// webview (no server, no
    // History API base), so PathLocationStrategy can't match `wails://localhost/…`
    // and the outlet renders nothing. Hash routing reads the fragment instead.
    provideRouter(routes, withHashLocation()),
    provideStore(),
    provideState(desktopFeature),
    provideEffects([DesktopEffects]),
    provideRouterStore(),
    provideStoreDevtools({ maxAge: 25, logOnly: !isDevMode() }),
    provideAppInitializer(() => inject(DesktopMcpService).ready),
  ],
};
