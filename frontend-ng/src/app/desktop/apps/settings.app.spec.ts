import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideMockStore } from '@ngrx/store/testing';
import { Store } from '@ngrx/store';
import { PreferencesService } from '../preferences.service';
import { WindowManagerService } from '../window-manager.service';
import { desktopControlsActions } from '../../store/desktop-controls.actions';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
} from '../../store/desktop-controls.reducer';
import { SettingsApp } from './settings.app';

describe('SettingsApp desktop controls', () => {
  beforeEach(() => {
    const prefs = {
      lang: signal('en'),
      design: signal<'lethean' | 'custom'>('lethean'),
      customName: signal('Host UK'),
      customHue: signal(305),
      wallpaper: signal<'aurora' | 'dusk' | 'mist' | 'graphite'>('aurora'),
      bar: signal<'top' | 'right' | 'bottom' | 'left'>('bottom'),
      showIcons: signal(true),
      showWidgets: signal(true),
      mode: signal<'dark' | 'light'>('dark'),
      reduceMotion: signal(false),
    };
    const wm = {
      view: signal<'desktop' | 'shell' | 'device'>('desktop'),
      device: signal<'small' | 'large' | 'full'>('small'),
      setSub: vi.fn(),
      setView: vi.fn(),
      setDevice: vi.fn(),
    };
    TestBed.configureTestingModule({
      imports: [SettingsApp],
      providers: [
        provideMockStore({
          selectors: [
            {
              selector: selectDesktopControlGroups,
              value: [
                {
                  name: 'Theme',
                  controls: [
                    {
                      key: 'desktop.theme.interface',
                      group: 'Theme',
                      label: 'Interface theme',
                      description: 'Desktop colour mode.',
                      kind: 'select',
                      value: 'dark',
                      defaultValue: 'dark',
                      configured: false,
                      live: true,
                      restartRequired: false,
                      choices: ['dark', 'light'],
                    },
                  ],
                },
                {
                  name: 'Single instance',
                  controls: [
                    {
                      key: 'desktop.single_instance.enabled',
                      group: 'Single instance',
                      label: 'Single-instance hand-off',
                      description: 'Hand later launches to the running process.',
                      kind: 'toggle',
                      value: true,
                      defaultValue: true,
                      configured: false,
                      live: false,
                      restartRequired: true,
                    },
                  ],
                },
              ],
            },
            { selector: desktopControlsFeature.selectLoading, value: false },
            { selector: desktopControlsFeature.selectError, value: null },
            {
              selector: desktopControlsFeature.selectConfigPath,
              value: '/Users/test/Lethean/conf/lthn.yaml',
            },
            { selector: desktopControlsFeature.selectSavingKeys, value: [] },
          ],
        }),
        { provide: PreferencesService, useValue: prefs },
        { provide: WindowManagerService, useValue: wm },
      ],
    });
  });

  it('loads and renders the grouped persisted control panel', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', {
      id: 'settings',
      app: 'settings',
      sub: 'interface',
      systab: '',
      x: 0,
      y: 0,
      w: 780,
      h: 560,
      z: 1,
      min: false,
      max: false,
    });
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.load());
    expect(fixture.nativeElement.textContent).toContain('Desktop controls');
    expect(fixture.nativeElement.textContent).toContain('Theme');
    expect(fixture.nativeElement.textContent).toContain('Interface theme');
    expect(fixture.nativeElement.textContent).toContain('Restart required');
  });

  it('dispatches a selected control value through NgRx', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', {
      id: 'settings',
      app: 'settings',
      sub: 'interface',
      systab: '',
      x: 0,
      y: 0,
      w: 780,
      h: 560,
      z: 1,
      min: false,
      max: false,
    });
    await fixture.whenStable();

    const select = fixture.nativeElement.querySelector(
      '[data-control-key="desktop.theme.interface"] select',
    ) as HTMLSelectElement;
    select.value = 'light';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.setControl({
        key: 'desktop.theme.interface',
        value: 'light',
      }),
    );
  });
});
