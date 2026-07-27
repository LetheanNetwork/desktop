// ─────────────────────────────────────────────────────────────────────────
// preferences.service.ts — OS/UI preferences, the sibling of WindowManagerService.
//
// Window *state* lives in WindowManagerService; OS *preferences* live here:
// theme, wallpaper, brand/design (incl. a custom accent), language, and the
// desktop toggles (icons, widgets, reduce-motion). The Settings app and the
// shell both read/write this one service, so there's a single prefs source and
// one place to persist/sync (point persist()/restore() at the CoreGo config
// store — this maps cleanly onto corego/pkg/config's Defaults→File→Env→Set).
// ─────────────────────────────────────────────────────────────────────────
import { DOCUMENT } from '@angular/common';
import { Injectable, effect, inject, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlSnapshot, DesktopControlValue } from '../store/desktop-controls.models';
import { DESKTOP_STORAGE } from '../store/storage.service';

export type Mode = 'dark' | 'light';
export type Brand = 'lethean' | 'hostuk';
export type Design = 'lethean' | 'custom';
export type Wallpaper = 'aurora' | 'dusk' | 'mist' | 'graphite';
export type TaskbarEdge = 'top' | 'right' | 'bottom' | 'left';

const KEY = 'lthn.prefs';

@Injectable({ providedIn: 'root' })
export class PreferencesService {
  private readonly document = inject(DOCUMENT);
  private readonly storage = inject(DESKTOP_STORAGE);
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
    if (this.connection.offline()) this.restore();
    // One effect keeps the DOM/token attributes in sync. Browser persistence
    // belongs exclusively to the explicit offline demo provider.
    effect(() => {
      const root = this.document.documentElement,
        body = this.document.body;
      root.setAttribute('data-brand', this.brand());
      body.setAttribute('data-brand', this.brand());
      if (this.connection.offline()) this.persist();
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

  persist() {
    if (!this.connection.offline()) return;
    try {
      this.storage.setItem(
        KEY,
        JSON.stringify({
          bar: this.bar(),
          mode: this.mode(),
          brand: this.brand(),
          design: this.design(),
          customHue: this.customHue(),
          customName: this.customName(),
          wallpaper: this.wallpaper(),
          lang: this.lang(),
          showIcons: this.showIcons(),
          showWidgets: this.showWidgets(),
          reduceMotion: this.reduceMotion(),
        }),
      );
    } catch {}
  }
  restore() {
    if (!this.connection.offline()) return;
    try {
      const s = JSON.parse(this.storage.getItem(KEY) || 'null');
      if (!s) return;
      if (s.bar) this.bar.set(s.bar);
      if (s.mode) this.mode.set(s.mode);
      if (s.brand) this.brand.set(s.brand);
      if (s.design) this.design.set(s.design);
      if (typeof s.customHue === 'number') this.customHue.set(s.customHue);
      if (s.customName) this.customName.set(s.customName);
      if (s.wallpaper) this.wallpaper.set(s.wallpaper);
      if (s.lang) this.lang.set(s.lang);
      if (typeof s.showIcons === 'boolean') this.showIcons.set(s.showIcons);
      if (typeof s.showWidgets === 'boolean') this.showWidgets.set(s.showWidgets);
      if (typeof s.reduceMotion === 'boolean') this.reduceMotion.set(s.reduceMotion);
    } catch {}
  }
}

function isOneOf<const T extends string>(
  value: DesktopControlValue | undefined,
  choices: readonly T[],
): value is T {
  return typeof value === 'string' && choices.includes(value as T);
}
