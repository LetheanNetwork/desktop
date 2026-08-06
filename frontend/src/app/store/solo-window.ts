// SPDX-License-Identifier: EUPL-1.2

// Is this document the solo window of a torn-off application?
//
// A tear-off opens a second native window on the same single-page
// application, at /#/w/<app>. Both windows would otherwise author the one
// persisted shell session: the solo window launches its application into the
// session, the shell — which has just closed the window it handed over —
// reads that record back on its next save conflict, and the application
// reappears in the shell it had left. The move undoes itself.
//
// So a solo window reads one application and writes nothing durable. The
// shell stays the only author of the session it owns.

import { DOCUMENT } from '@angular/common';
import { InjectionToken, inject } from '@angular/core';

/**
 * Whether a document URL addresses the solo application route.
 *
 * Read from the URL rather than the Router: the persistence effects are
 * constructed and fire before the first navigation completes, and a window
 * opened at /#/w/<app> is a solo window from the moment it exists.
 *
 *     isSoloWindowUrl('wails://localhost/#/w/telemetry') // true
 *     isSoloWindowUrl('wails://localhost/#/')            // false
 */
export function isSoloWindowUrl(url: string): boolean {
  const marker = url.indexOf('#');
  if (marker < 0) return false;
  return url.slice(marker + 1).startsWith('/w/');
}

/** True in a torn-off application's own native window. */
export const SOLO_APP_WINDOW = new InjectionToken<boolean>('SOLO_APP_WINDOW', {
  providedIn: 'root',
  factory: () => isSoloWindowUrl(inject(DOCUMENT).defaultView?.location.href ?? ''),
});
