import { Injectable, computed, inject, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

/**
 * Tearing an application out of the shell and into its own native window, and
 * docking it back in.
 *
 * The desktop is a windowing shell inside one frameless native window, so an
 * application that leaves it needs a second native window. Only Go can open
 * one, and it will only do so for an application it recognises: this bridge
 * names an application from the catalogue, never a URL, and the Go side
 * assembles the solo route itself.
 *
 * A tear-off is a MOVE. The caller closes the in-shell window only after this
 * reports success, because a failed native open followed by a closed in-shell
 * window is an application that simply vanished.
 *
 * Docking back is the same move mirrored, and carries the same rule: the solo
 * window closes only after Go has accepted the request and put the adoption on
 * the event bus for the shell to pick up. The solo window never writes the
 * shell's session itself — the shell is that session's only author.
 */
const APP_WINDOW_SERVICE = 'dappco.re/lthn/desktop/pkg/desktop.AppWindowService';

export const APP_WINDOW_METHODS = {
  openApp: `${APP_WINDOW_SERVICE}.OpenApp`,
  dockApp: `${APP_WINDOW_SERVICE}.DockApp`,
} as const;

/**
 * The event Go broadcasts when a solo window asks to be adopted back into the
 * shell. Emitted from Go rather than from the solo window so the application
 * identifier is validated against the same whitelist a tear-off is, and so the
 * shell window is already revealed by the time the request lands.
 */
export const DOCK_APP_EVENT = 'lthn:app:dock';

/** What the shell knows about the window it is handing over. */
export interface AppWindowRequest {
  readonly app: string;
  readonly pane?: string;
  readonly width?: number;
  readonly height?: number;
}

@Injectable({ providedIn: 'root' })
export class DesktopAppWindowBridgeService {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);

  private readonly _lastError = signal('');

  /**
   * Why the last tear-off did nothing.
   *
   * A control that silently fails is the worst version of this: the window
   * stays where it was and there is nothing to look at. The shell reads this
   * to raise a notification instead of swallowing the reason.
   */
  readonly lastError = this._lastError.asReadonly();

  /**
   * Whether a native window can actually be opened.
   *
   * False in the browser demo, where there is no second window to open. The
   * titlebar renders the control disabled rather than hiding it, so the bar
   * does not change shape between demo and native.
   */
  readonly available = computed(() => !this.connection.offline());

  /** Move one application into its own native window. */
  async openApp(request: AppWindowRequest): Promise<boolean> {
    return await this.move('openApp', APP_WINDOW_METHODS.openApp, request);
  }

  /**
   * Ask the shell to take one application back.
   *
   * Go validates the application, reveals the shell window, and puts the
   * adoption on the event bus; the shell listens and launches the application
   * into the session it owns. Only then may the caller close the solo window —
   * a solo window that closed first would take the application with it.
   */
  async dockApp(request: AppWindowRequest): Promise<boolean> {
    return await this.move('dockApp', APP_WINDOW_METHODS.dockApp, request);
  }

  private async move(label: string, method: string, request: AppWindowRequest): Promise<boolean> {
    if (!request.app) {
      this._lastError.set(`${label}: no application named`);
      return false;
    }
    if (!this.available()) {
      this._lastError.set(`${label}: no native window host`);
      return false;
    }

    try {
      await this.surface.call(method, [asRequestPayload(request)]);
      this._lastError.set('');
      return true;
    } catch (error) {
      this._lastError.set(String(error));
      return false;
    }
  }
}

/**
 * Only whole positive pixels cross the boundary. Go rejects a size outside its
 * usable range outright, and a fractional or negative measurement is a bug on
 * this side rather than a size worth arguing about, so it is dropped and the
 * tear-off window keeps its own default.
 */
function asRequestPayload(request: AppWindowRequest): Record<string, unknown> {
  const width = roundedDimension(request.width);
  const height = roundedDimension(request.height);
  return {
    app: request.app,
    pane: request.pane ?? '',
    width: width && height ? width : 0,
    height: width && height ? height : 0,
  };
}

function roundedDimension(value: number | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.round(value) : 0;
}
