import { Injectable, signal } from '@angular/core';
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

@Injectable({ providedIn: 'root' })
export class DesktopWindowBridgeService {
  private readonly connection = inject(ConnectionManagerService);

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
      this._maximised.set(Boolean(await Window.IsMaximised()));
    } catch (error) {
      // A window that cannot answer is not a reason to break the bar, but it
      // is a reason to say so.
      this._lastError.set(`isMaximised: ${String(error)}`);
    }
  }

  async minimise(): Promise<void> {
    await this.run((Window) => Window.Minimise());
  }

  /**
   * Maximise or restore, whichever the window is not.
   *
   * One button, because that is what every platform gives a person here, and
   * two would need the state to be right before the first click.
   */
  async toggleMaximise(): Promise<void> {
    await this.run((Window) => Window.ToggleMaximise());
    await this.refreshMaximised();
  }

  async close(): Promise<void> {
    await this.run((Window) => Window.Close());
  }

  private async run(
    action: (window: typeof import('@wailsio/runtime').Window) => unknown,
  ): Promise<void> {
    if (!this._available()) return;

    try {
      const { Window } = await import('@wailsio/runtime');
      await action(Window);
      this._lastError.set('');
    } catch (error) {
      this._lastError.set(String(error));
    }
  }
}
