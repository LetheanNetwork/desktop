import { DOCUMENT } from '@angular/common';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlSnapshot, DesktopControlValue } from '../store/desktop-controls.models';
import { DESKTOP_STORAGE } from '../store/storage.service';
import { PreferencesService } from './preferences.service';

const preferenceValues: ReadonlyArray<readonly [string, DesktopControlValue]> = [
  ['desktop.shell.taskbar_edge', 'left'],
  ['desktop.shell.show_icons', false],
  ['desktop.shell.show_widgets', false],
  ['desktop.theme.interface', 'light'],
  ['desktop.theme.brand', 'hostuk'],
  ['desktop.theme.design', 'custom'],
  ['desktop.theme.custom_hue', 190],
  ['desktop.theme.custom_name', 'Calm'],
  ['desktop.theme.wallpaper', 'mist'],
  ['desktop.theme.reduce_motion', true],
  ['desktop.locale.language', 'cy'],
];

const preferenceSnapshot: DesktopControlSnapshot = {
  revision: '1',
  controls: preferenceValues.map(([key, value]) => ({
    key,
    group: 'Theme',
    label: String(key),
    description: String(key),
    kind:
      typeof value === 'boolean'
        ? ('toggle' as const)
        : typeof value === 'number'
          ? ('number' as const)
          : ('text' as const),
    value,
    defaultValue: value,
    configured: true,
    live: true,
    restartRequired: false,
  })),
};

describe('PreferencesService', () => {
  const offline = signal(false);
  let values: Map<string, string>;
  let storage: Storage;

  beforeEach(() => {
    offline.set(false);
    values = new Map();
    storage = {
      get length() {
        return values.size;
      },
      clear: vi.fn(() => values.clear()),
      getItem: vi.fn((key: string) => values.get(key) ?? null),
      key: vi.fn((index: number) => [...values.keys()][index] ?? null),
      removeItem: vi.fn((key: string) => values.delete(key)),
      setItem: vi.fn((key: string, value: string) => values.set(key, value)),
    };
    TestBed.configureTestingModule({
      providers: [
        PreferencesService,
        { provide: DESKTOP_STORAGE, useValue: storage },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
  });

  it('projects the authoritative connected snapshot without browser persistence', async () => {
    values.set(
      'lthn.prefs',
      JSON.stringify({ mode: 'dark', wallpaper: 'graphite', showIcons: true }),
    );
    const service = TestBed.inject(PreferencesService);

    service.applySnapshot(preferenceSnapshot);
    TestBed.flushEffects();

    expect(service.bar()).toBe('left');
    expect(service.mode()).toBe('light');
    expect(service.brand()).toBe('hostuk');
    expect(service.design()).toBe('custom');
    expect(service.customHue()).toBe(190);
    expect(service.customName()).toBe('Calm');
    expect(service.wallpaper()).toBe('mist');
    expect(service.lang()).toBe('cy');
    expect(service.showIcons()).toBe(false);
    expect(service.showWidgets()).toBe(false);
    expect(service.reduceMotion()).toBe(true);
    expect(storage.setItem).not.toHaveBeenCalled();
  });

  it('writes the custom accent ramp onto any screen that carries it', () => {
    const service = TestBed.inject(PreferencesService);
    const screen = document.createElement('div');

    service.applySnapshot(preferenceSnapshot);
    service.applyDesignTo(screen);

    expect(screen.style.getPropertyValue('--brand-500')).toBe('oklch(0.54 0.16 190)');
    expect(screen.style.getPropertyValue('--brand-50')).toBe('oklch(0.96 0.02 190)');
    expect(screen.style.getPropertyValue('--brand-900')).toBe('oklch(0.22 0.075 190)');
    expect(screen.style.getPropertyValue('--brand-name')).toBe("'Calm'");
  });

  it('clears the custom accent ramp when the design returns to the stylesheet', () => {
    const service = TestBed.inject(PreferencesService);
    const screen = document.createElement('div');

    service.applySnapshot(preferenceSnapshot);
    service.applyDesignTo(screen);
    service.design.set('lethean');
    service.applyDesignTo(screen);

    expect(screen.style.getPropertyValue('--brand-500')).toBe('');
    expect(screen.style.getPropertyValue('--brand-name')).toBe('');
    expect(screen.getAttribute('style')).toBe('');
  });

  it('leaves offline persistence to the controls repository', () => {
    offline.set(true);
    values.set('lthn.prefs', JSON.stringify({ mode: 'dark', wallpaper: 'dusk' }));

    const service = TestBed.inject(PreferencesService);
    service.applySnapshot(preferenceSnapshot);
    TestBed.flushEffects();

    expect(service.mode()).toBe('light');
    expect(service.wallpaper()).toBe('mist');
    expect(storage.setItem).not.toHaveBeenCalled();
  });
});
