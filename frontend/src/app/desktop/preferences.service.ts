// ─────────────────────────────────────────────────────────────────────────
// preferences.service.ts — OS/UI preferences, the sibling of WindowManagerService.
//
// Window *state* lives in WindowManagerService; OS *preferences* live here:
// theme, wallpaper, brand/design (incl. a custom accent), language, and the
// desktop toggles (icons, widgets, reduce-motion). NgRx projects committed
// snapshots into these renderer signals; persistence stays in the connected
// appconfig Medium or the explicit offline controls repository.
// ─────────────────────────────────────────────────────────────────────────
import { DOCUMENT } from '@angular/common';
import { Injectable, effect, inject, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlSnapshot, DesktopControlValue } from '../store/desktop-controls.models';

export type Mode = 'dark' | 'light';
export type Brand = 'lethean' | 'hostuk';
export type Design = 'lethean' | 'custom';
export type Wallpaper = 'aurora' | 'dusk' | 'mist' | 'graphite';
export type TaskbarEdge = 'top' | 'right' | 'bottom' | 'left';

@Injectable({ providedIn: 'root' })
export class PreferencesService {
  private readonly document = inject(DOCUMENT);
  private readonly connection = inject(ConnectionManagerService);
  readonly offline = this.connection.offline;

  readonly bar = signal<TaskbarEdge>('bottom');
  readonly mode = signal<Mode>('dark');
  readonly brand = signal<Brand>('lethean');
  readonly design = signal<Design>('lethean');
  readonly customHue = signal<number>(305); // Vi-purple custom accent (oklch hue)
  readonly customName = signal<string>(
    $localize`:Default custom design name@@preferences.customName.default:Host UK`,
  );
  readonly wallpaper = signal<Wallpaper>('aurora');
  readonly lang = signal<string>('en');
  readonly showIcons = signal<boolean>(true);
  readonly showWidgets = signal<boolean>(true);
  readonly reduceMotion = signal<boolean>(false);

  constructor() {
    // One effect keeps the DOM/token attributes in sync. Persistence belongs
    // exclusively to the selected connected or offline controls provider.
    effect(() => {
      const root = this.document.documentElement,
        body = this.document.body;
      root.setAttribute('data-brand', this.brand());
      body.setAttribute('data-brand', this.brand());
    });
  }

  /** Project one committed appconfig snapshot into renderer-owned signals. */
  applySnapshot(snapshot: DesktopControlSnapshot): void {
    const value = (key: string): DesktopControlValue | undefined =>
      snapshot.controls.find((control) => control.key === key)?.value;

    const bar = value('desktop.shell.taskbar_edge');
    if (isOneOf(bar, ['top', 'right', 'bottom', 'left'])) this.bar.set(bar);

    const mode = value('desktop.theme.interface');
    if (isOneOf(mode, ['dark', 'light'])) this.mode.set(mode);

    const brand = value('desktop.theme.brand');
    if (isOneOf(brand, ['lethean', 'hostuk'])) this.brand.set(brand);

    const design = value('desktop.theme.design');
    if (isOneOf(design, ['lethean', 'custom'])) this.design.set(design);

    const customHue = value('desktop.theme.custom_hue');
    if (
      typeof customHue === 'number' &&
      Number.isFinite(customHue) &&
      customHue >= 0 &&
      customHue <= 360
    ) {
      this.customHue.set(customHue);
    }

    const customName = value('desktop.theme.custom_name');
    if (typeof customName === 'string' && customName.length <= 2_048) {
      this.customName.set(customName);
    }

    const wallpaper = value('desktop.theme.wallpaper');
    if (isOneOf(wallpaper, ['aurora', 'dusk', 'mist', 'graphite'])) {
      this.wallpaper.set(wallpaper);
    }

    const language = value('desktop.locale.language');
    if (isOneOf(language, ['en', 'cy', 'de', 'es', 'fr', 'ja'])) {
      this.lang.set(language);
    }

    const showIcons = value('desktop.shell.show_icons');
    if (typeof showIcons === 'boolean') this.showIcons.set(showIcons);

    const showWidgets = value('desktop.shell.show_widgets');
    if (typeof showWidgets === 'boolean') this.showWidgets.set(showWidgets);

    const reduceMotion = value('desktop.theme.reduce_motion');
    if (typeof reduceMotion === 'boolean') this.reduceMotion.set(reduceMotion);
  }

  /** Design label shown in About / Settings. */
  designName() {
    return this.design() === 'custom'
      ? this.customName() || $localize`:Design name@@preferences.design.custom:Custom`
      : this.brand() === 'hostuk'
        ? $localize`:Design name@@preferences.design.hostUk:Host UK`
        : $localize`:Design name@@preferences.design.lethean:Lethean`;
  }

  /** Apply prefs that live on the OS-screen element (mode/wallpaper are scoped there,
   *  not global) — call from the shell with its #os element. */
  applyTo(os: HTMLElement) {
    os.setAttribute('data-wall', this.wallpaper());
    if (this.mode() === 'light') os.setAttribute('data-mode', 'light');
    else os.removeAttribute('data-mode');
    os.classList.toggle('no-icons', !this.showIcons());
    os.classList.toggle('no-widgets', !this.showWidgets());
    os.classList.toggle('reduce-motion', this.reduceMotion());
  }
}

function isOneOf<const T extends string>(
  value: DesktopControlValue | undefined,
  choices: readonly T[],
): value is T {
  return typeof value === 'string' && choices.includes(value as T);
}
