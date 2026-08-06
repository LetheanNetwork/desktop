import { DOCUMENT } from '@angular/common';
import { InjectionToken, Injectable, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { inject } from '@angular/core';

/**
 * The native window's own controls — close, minimise, maximise.
 *
 * The application window is frameless, so the operating system paints no
 * title bar and no buttons. Everything a person uses to move, resize or close
 * the window is drawn by us, which means these three have to exist and have to
 * be wired to the real window rather than to the inner desktop's window
 * manager. Those are different things: `Minimise Window` in the desktop store
 * minimises a window *inside* the canvas.
 *
 * Where they sit is a platform convention, not a preference:
 *
 *   macOS, Linux   left, before anything else
 *   Windows        right, beside the tray indicators
 *
 * Getting that backwards is immediately wrong to anyone using the machine, so
 * the side is decided here from the runtime rather than guessed in CSS.
 */
export type WindowControlSide = 'left' | 'right';

/**
 * Native window names, mirroring the constants in go/pkg/desktop/windows.go.
 *
 * Go names every window it opens; these are the same names read back from the
 * route the document is showing, because the renderer has to say which window
 * it means. See NATIVE_WINDOW_NAME.
 */
const MAIN_WINDOW_NAME = 'app';
const TRAY_PANEL_WINDOW_NAME = 'tray-panel';
const SOLO_WINDOW_NAME_PREFIX = 'app-view-';

/**
 * Which native window a document is.
 *
 * Wails' `Window` object is documented as "the window within which the script
 * is running", and over the platform WebView that is true: the macOS scheme
 * handler is registered per window and stamps `x-wails-window-id` and
 * `x-wails-window-name` onto every request it forwards
 * (wails/v3 pkg/application/application_darwin.go, processURLRequest).
 *
 * This application does not use that path. ConnectionManagerService installs a
 * custom transport over the Lethean WebSocket, and a custom transport carries
 * only what the caller puts in the message — `webviewWindowName`, which the
 * unnamed `Window` object leaves empty. Go then falls through to
 *
 *     // No window specified - return the first available window
 *     windows := globalApplication.Window.GetAll()
 *
 * (wails/v3 pkg/application/messageprocessor.go, getTargetWindow), which is the
 * shell. Unnamed calls from a torn-off application would therefore minimise,
 * maximise or close the desktop it had just left.
 *
 * So the window names itself. The name is derived from the route rather than
 * asked for over the runtime, because asking would be the same mis-targeted
 * call.
 *
 *     nativeWindowNameForUrl('wails://localhost/#/w/telemetry') // 'app-view-telemetry'
 *     nativeWindowNameForUrl('wails://localhost/#/tray')        // 'tray-panel'
 *     nativeWindowNameForUrl('wails://localhost/#/')            // 'app'
 */
export function nativeWindowNameForUrl(url: string): string {
  const marker = url.indexOf('#');
  const route = marker < 0 ? '' : url.slice(marker + 1);
  if (route === '/tray' || route.startsWith('/tray/') || route.startsWith('/tray?')) {
    return TRAY_PANEL_WINDOW_NAME;
  }
  if (!route.startsWith('/w/')) return MAIN_WINDOW_NAME;
  const app = route.slice('/w/'.length).split(/[/?]/u)[0];
  return app ? SOLO_WINDOW_NAME_PREFIX + app : MAIN_WINDOW_NAME;
}

/** The name Go gave the native window this document is rendering in. */
export const NATIVE_WINDOW_NAME = new InjectionToken<string>('NATIVE_WINDOW_NAME', {
  providedIn: 'root',
  factory: () => nativeWindowNameForUrl(inject(DOCUMENT).defaultView?.location.href ?? ''),
});

@Injectable({ providedIn: 'root' })
export class DesktopWindowBridgeService {
  private readonly connection = inject(ConnectionManagerService);

  /**
   * The window these controls drive. Every runtime call is addressed to it by
   * name, so a second window's title bar cannot reach the first one's.
   */
  readonly windowName = inject(NATIVE_WINDOW_NAME);

  private readonly _maximised = signal(false);

  /** Whether the native window is currently maximised. */
  readonly maximised = this._maximised.asReadonly();

  private readonly _side = signal<WindowControlSide>('left');

  /** Which side of the bar the controls belong on for this platform. */
  readonly side = this._side.asReadonly();

  private readonly _lastError = signal('');

  /**
   * Why the last control did nothing.
   *
   * A title-bar button that silently fails is the worst version of this: the
   * window stays put and there is nothing to look at. Swallowing the error was
   * how the first cut of this shipped broken.
   */
  readonly lastError = this._lastError.asReadonly();

  private readonly _available = signal(false);

  /**
   * Whether a native window is actually behind these controls.
   *
   * False in the browser demo, where the buttons would be lying: there is no
   * window to close. Callers render them disabled rather than hiding them, so
   * the layout does not move between demo and native.
   */
  readonly available = this._available.asReadonly();

  constructor() {
    void this.detect();
  }

  private async detect(): Promise<void> {
    if (this.connection.offline()) return;

    try {
      const { System } = await import('@wailsio/runtime');
      // Windows puts its caption buttons on the right; the others on the left.
      const windows = System.IsWindows();
      this._side.set(windows ? 'right' : 'left');

      // The controls are round on macOS and Linux and square in the corner on
      // Windows. That is a whole different shape rather than a tweak, so the
      // platform is published to CSS instead of being branched on per rule.
      if (typeof document !== 'undefined') {
        document.documentElement.classList.toggle('platform-windows', windows);
        document.documentElement.classList.toggle('platform-mac', System.IsMac());
      }

      this._available.set(true);
      await this.refreshMaximised();
    } catch (error) {
      // No runtime: the browser demo. Leave the defaults and stay unavailable.
      this._lastError.set(`detect: ${String(error)}`);
      this._available.set(false);
    }
  }

  /** Re-read the maximise state, so the middle button can show the right thing. */
  async refreshMaximised(): Promise<void> {
    if (!this._available()) return;

    try {
      const { Window } = await import('@wailsio/runtime');
      this._maximised.set(Boolean(await Window.Get(this.windowName).IsMaximised()));
    } catch (error) {
      // A window that cannot answer is not a reason to break the bar, but it
      // is a reason to say so.
      this._lastError.set(`isMaximised: ${String(error)}`);
    }
  }

  async minimise(): Promise<void> {
    await this.run((window) => window.Minimise());
  }

  /**
   * Maximise or restore, whichever the window is not.
   *
   * One button, because that is what every platform gives a person here, and
   * two would need the state to be right before the first click.
   */
  async toggleMaximise(): Promise<void> {
    await this.run((window) => window.ToggleMaximise());
    await this.refreshMaximised();
  }

  async close(): Promise<void> {
    await this.run((window) => window.Close());
  }

  private async run(
    action: (window: typeof import('@wailsio/runtime').Window) => unknown,
  ): Promise<void> {
    if (!this._available()) return;

    try {
      const { Window } = await import('@wailsio/runtime');
      await action(Window.Get(this.windowName));
      this._lastError.set('');
    } catch (error) {
      this._lastError.set(String(error));
    }
  }
}
