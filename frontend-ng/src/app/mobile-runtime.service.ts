import { DOCUMENT } from '@angular/common';
import { InjectionToken, Service, inject, signal } from '@angular/core';
import { Events, System } from '@wailsio/runtime';
import { WindowManagerService } from './desktop/window-manager.service';

export type LetheanPlatform = 'web' | 'darwin' | 'windows' | 'linux' | 'ios' | 'ipad' | 'android';

export interface MobileSafeArea {
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
  readonly left: number;
}

export interface MobileBatteryState {
  readonly level: number | null;
  readonly state: string;
  readonly charging: boolean;
  readonly lowPowerMode: boolean;
}

export interface MobileNetworkState {
  readonly connected: boolean;
  readonly type: string;
}

export interface MobileRuntimeEvent {
  readonly name: string;
  readonly payload: unknown;
  readonly at: string;
}

export interface MobileRuntimeTransport {
  platform(): LetheanPlatform;
  on(name: string, handler: (payload: unknown) => void): () => void;
  emit(name: string, payload: Record<string, unknown>): Promise<void>;
}

function runtimePlatform(): LetheanPlatform {
  if (System.IsIOS()) {
    const navigatorPlatform = globalThis.navigator?.platform ?? '';
    const userAgent = globalThis.navigator?.userAgent ?? '';
    const iPad =
      /iPad/i.test(userAgent) ||
      (navigatorPlatform === 'MacIntel' && (globalThis.navigator?.maxTouchPoints ?? 0) > 1);
    return iPad ? 'ipad' : 'ios';
  }
  if (System.IsAndroid()) return 'android';
  if (System.IsMac()) return 'darwin';
  if (System.IsWindows()) return 'windows';
  if (System.IsLinux()) return 'linux';
  return 'web';
}

export const MOBILE_RUNTIME_TRANSPORT = new InjectionToken<MobileRuntimeTransport>(
  'MOBILE_RUNTIME_TRANSPORT',
  {
    providedIn: 'root',
    factory: () => ({
      platform: runtimePlatform,
      on(name, handler): () => void {
        return Events.On(name, (event) => handler(event.data));
      },
      async emit(name, payload): Promise<void> {
        await Events.Emit(name, payload);
      },
    }),
  },
);

const EMPTY_SAFE_AREA: MobileSafeArea = { top: 0, right: 0, bottom: 0, left: 0 };
const MAX_EVENT_LOG = 64;

@Service()
export class MobileRuntimeService {
  private readonly document = inject(DOCUMENT);
  private readonly transport = inject(MOBILE_RUNTIME_TRANSPORT);
  private readonly windows = inject(WindowManagerService);
  private readonly off: Array<() => void> = [];
  private started = false;

  private readonly _platform = signal<LetheanPlatform>('web');
  readonly platform = this._platform.asReadonly();

  private readonly _foreground = signal(true);
  readonly foreground = this._foreground.asReadonly();

  private readonly _active = signal(true);
  readonly active = this._active.asReadonly();

  private readonly _locked = signal(false);
  readonly locked = this._locked.asReadonly();

  private readonly _lowMemoryPulses = signal(0);
  readonly lowMemoryPulses = this._lowMemoryPulses.asReadonly();

  private readonly _safeArea = signal<MobileSafeArea>(EMPTY_SAFE_AREA);
  readonly safeArea = this._safeArea.asReadonly();

  private readonly _battery = signal<MobileBatteryState | null>(null);
  readonly battery = this._battery.asReadonly();

  private readonly _network = signal<MobileNetworkState | null>(null);
  readonly network = this._network.asReadonly();

  private readonly _orientation = signal('');
  readonly orientation = this._orientation.asReadonly();

  private readonly _brightness = signal<number | null>(null);
  readonly brightness = this._brightness.asReadonly();

  private readonly _appInfo = signal<Record<string, unknown> | null>(null);
  readonly appInfo = this._appInfo.asReadonly();

  private readonly _events = signal<readonly MobileRuntimeEvent[]>([]);
  readonly events = this._events.asReadonly();

  readonly ready = this.initialise();

  private async initialise(): Promise<void> {
    if (this.started) return;
    this.started = true;

    const platform = this.transport.platform();
    this._platform.set(platform);
    this.document.documentElement.dataset['platform'] = platform;

    if (platform === 'ios' || platform === 'ipad' || platform === 'android') {
      this.windows.setView('device');
      this.windows.setDevice(
        platform === 'ipad' || this.document.documentElement.clientWidth >= 768
          ? 'large'
          : 'small',
      );
    }

    this.listen('lthn:app:background', () => this._foreground.set(false));
    this.listen('lthn:app:foreground', () => this._foreground.set(true));
    this.listen('lthn:app:inactive', () => this._active.set(false));
    this.listen('lthn:app:active', () => this._active.set(true));
    this.listen('lthn:system:lock', (payload) => {
      const value = asRecord(payload);
      this._locked.set(value?.['locked'] === true);
    });
    this.listen('lthn:system:low-memory', () => {
      this._lowMemoryPulses.update((value) => value + 1);
    });
    this.listen('lthn:system:battery', (payload) => this.setBattery(payload));
    this.listen('lthn:system:network', (payload) => this.setNetwork(payload));
    this.listen('common:power', (payload) => this.setBattery(payload));
    this.listen('common:network', (payload) => this.setNetwork(payload));
    this.listen('common:safeArea', (payload) => this.setSafeArea(payload));
    this.listen('common:orientation', (payload) => {
      const value = asRecord(payload)?.['orientation'];
      this._orientation.set(typeof value === 'string' ? value : '');
    });
    this.listen('common:brightness', (payload) => {
      const value = asRecord(payload)?.['value'];
      this._brightness.set(typeof value === 'number' && Number.isFinite(value) ? value : null);
    });
    this.listen('common:appInfo', (payload) => {
      this._appInfo.set(asRecord(payload));
    });

    if (platform === 'ios' || platform === 'ipad' || platform === 'android') {
      await Promise.all([
        this.emit('common:getSafeArea'),
        this.emit('common:getPower'),
        this.emit('common:getNetwork'),
        this.emit('common:getAppInfo'),
        this.emit('common:getOrientation'),
      ]);
    }
  }

  destroy(): void {
    for (const unsubscribe of this.off.splice(0)) unsubscribe();
    this.started = false;
  }

  share(text: string, url = ''): Promise<void> {
    return this.emit('common:share', { text, url });
  }

  openUrl(url: string): Promise<void> {
    return this.emit('common:openURL', { url });
  }

  setKeepAwake(enabled: boolean): Promise<void> {
    return this.emit('common:keepAwake', { enabled });
  }

  setTorch(enabled: boolean): Promise<void> {
    return this.emit('common:torch', { enabled });
  }

  setBrightness(value: number): Promise<void> {
    return this.emit('common:setBrightness', { value: Math.min(1, Math.max(0, value)) });
  }

  setOrientation(mode: string): Promise<void> {
    return this.emit('common:setOrientation', { mode });
  }

  setStatusBar(options: { readonly hidden?: boolean; readonly style?: string }): Promise<void> {
    return this.emit('common:setStatusBar', options);
  }

  authenticate(reason: string): Promise<void> {
    return this.emit('common:authenticate', { reason });
  }

  notify(title: string, body: string, delay = 0): Promise<void> {
    return this.emit('common:notify', { title, body, delay });
  }

  secureSet(key: string, value: string): Promise<void> {
    return this.emit('common:secureSet', { key, value });
  }

  secureGet(key: string): Promise<void> {
    return this.emit('common:secureGet', { key });
  }

  secureDelete(key: string): Promise<void> {
    return this.emit('common:secureDelete', { key });
  }

  haptic(type: string): Promise<void> {
    return this.emit('common:haptic', { type });
  }

  requestLocation(): Promise<void> {
    return this.emit('common:getLocation');
  }

  watchMotion(enabled: boolean): Promise<void> {
    return this.emit('common:watchMotion', { enabled });
  }

  watchProximity(enabled: boolean): Promise<void> {
    return this.emit('common:watchProximity', { enabled });
  }

  speak(text: string): Promise<void> {
    return this.emit('common:speak', { text });
  }

  stopSpeaking(): Promise<void> {
    return this.emit('common:stopSpeak');
  }

  watchKeyboard(enabled: boolean): Promise<void> {
    return this.emit('common:watchKeyboard', { enabled });
  }

  setScreenProtection(enabled: boolean): Promise<void> {
    return this.emit('common:setScreenProtect', { enabled });
  }

  capturePhoto(): Promise<void> {
    return this.emit('common:capturePhoto');
  }

  captureVideo(): Promise<void> {
    return this.emit('common:captureVideo');
  }

  startBackgroundWork(title: string, text: string): Promise<void> {
    return this.emit('common:startForegroundService', { title, text });
  }

  stopBackgroundWork(): Promise<void> {
    return this.emit('common:stopForegroundService');
  }

  private listen(name: string, handler: (payload: unknown) => void): void {
    this.off.push(
      this.transport.on(name, (payload) => {
        handler(payload);
        this._events.update((events) =>
          [
            {
              name,
              payload,
              at: new Date().toISOString(),
            },
            ...events,
          ].slice(0, MAX_EVENT_LOG),
        );
      }),
    );
  }

  private emit(name: string, payload: Record<string, unknown> = {}): Promise<void> {
    return this.transport.emit(name, payload);
  }

  private setSafeArea(payload: unknown): void {
    const value = asRecord(payload);
    const safeArea = {
      top: finiteNumber(value?.['top']),
      right: finiteNumber(value?.['right']),
      bottom: finiteNumber(value?.['bottom']),
      left: finiteNumber(value?.['left']),
    };
    this._safeArea.set(safeArea);

    const style = this.document.documentElement.style;
    style.setProperty('--safe-area-top', `${safeArea.top}px`);
    style.setProperty('--safe-area-right', `${safeArea.right}px`);
    style.setProperty('--safe-area-bottom', `${safeArea.bottom}px`);
    style.setProperty('--safe-area-left', `${safeArea.left}px`);
  }

  private setBattery(payload: unknown): void {
    const value = asRecord(payload);
    if (!value) return;
    const level = value['level'];
    const state = typeof value['state'] === 'string' ? value['state'] : '';
    const charging =
      value['charging'] === true || state === 'charging' || state === 'full';
    this._battery.set({
      level: typeof level === 'number' && Number.isFinite(level) && level >= 0 ? level : null,
      state,
      charging,
      lowPowerMode: value['lowPowerMode'] === true || value['lowPower'] === true,
    });
  }

  private setNetwork(payload: unknown): void {
    const value = asRecord(payload);
    if (!value) return;
    this._network.set({
      connected: value['connected'] === true || value['online'] === true,
      type: typeof value['type'] === 'string' ? value['type'] : 'unknown',
    });
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function finiteNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : 0;
}
