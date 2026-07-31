import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Store } from '@ngrx/store';
import { provideMockStore } from '@ngrx/store/testing';
import { desktopControlsActions } from '../../store/desktop-controls.actions';
import { ConnectionManagerService } from '../../connection-manager.service';
import { DesktopHostIntentService } from '../desktop-host-intent.service';
import { DesktopPermissionsBridgeService } from '../desktop-permissions-bridge.service';
import { DesktopControl } from '../../store/desktop-controls.models';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectDraftDesktopControls,
  selectHasDirtyDesktopControls,
} from '../../store/desktop-controls.reducer';
import { PreferencesService } from '../preferences.service';
import { WindowManagerService } from '../window-manager.service';
import { SettingsApp } from './settings.app';

const controls: readonly DesktopControl[] = [
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
  {
    key: 'desktop.permissions.notifications',
    group: 'Notifications',
    label: 'Web notifications',
    description: 'Policy for notification requests from WebView content.',
    kind: 'select',
    value: 'default',
    defaultValue: 'default',
    configured: false,
    live: false,
    restartRequired: true,
    choices: ['default', 'allow', 'deny'],
  },
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
];

const groups = [
  { name: 'Theme', controls: [controls[0]] },
  { name: 'Single instance', controls: [controls[2]] },
  { name: 'Notifications', controls: [controls[1]] },
];

const win = {
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
};

describe('SettingsApp desktop controls', () => {
  const hostIntents = {
    claimItems: vi.fn().mockReturnValue(null),
    onItems: vi.fn((app: 'files' | 'settings', consumer: (items: readonly unknown[]) => void) => {
      const items = hostIntents.claimItems(app);
      if (items) consumer(items);
      return vi.fn();
    }),
  };
  const permissions = {
    status: vi.fn().mockResolvedValue([
      { id: 'microphone', policy: 'default', host: 'unsupported' },
      { id: 'camera', policy: 'default', host: 'unsupported' },
      { id: 'geolocation', policy: 'default', host: 'unsupported' },
      { id: 'notifications', policy: 'default', host: 'granted' },
      { id: 'clipboard-read', policy: 'default', host: 'unsupported' },
    ]),
    request: vi.fn(),
  };

  beforeEach(() => {
    hostIntents.claimItems.mockReturnValue(null);
    permissions.status.mockResolvedValue([
      { id: 'microphone', policy: 'default', host: 'unsupported' },
      { id: 'camera', policy: 'default', host: 'unsupported' },
      { id: 'geolocation', policy: 'default', host: 'unsupported' },
      { id: 'notifications', policy: 'default', host: 'granted' },
      { id: 'clipboard-read', policy: 'default', host: 'unsupported' },
    ]);
    permissions.request.mockResolvedValue({
      id: 'notifications',
      policy: 'default',
      host: 'granted',
    });
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
            { selector: selectDraftDesktopControls, value: controls },
            { selector: selectDesktopControlGroups, value: groups },
            {
              selector: selectDirtyDesktopControlChanges,
              value: [{ key: 'desktop.theme.interface', value: 'light' }],
            },
            { selector: selectHasDirtyDesktopControls, value: true },
            { selector: desktopControlsFeature.selectLoading, value: false },
            {
              selector: desktopControlsFeature.selectError,
              value: 'Previous save failed safely.',
            },
            { selector: desktopControlsFeature.selectSaving, value: false },
            {
              selector: desktopControlsFeature.selectRestartSummary,
              value: 'Restart required for: Single-instance hand-off.',
            },
          ],
        }),
        { provide: PreferencesService, useValue: prefs },
        { provide: DesktopHostIntentService, useValue: hostIntents },
        { provide: DesktopPermissionsBridgeService, useValue: permissions },
        { provide: WindowManagerService, useValue: wm },
        {
          provide: ConnectionManagerService,
          useValue: { offline: signal(false).asReadonly() },
        },
      ],
    });
  });

  it('loads and renders the grouped draft with accessible status', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', win);

    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.load());
    expect(fixture.nativeElement.textContent).toContain('Desktop controls');
    expect(fixture.nativeElement.querySelector('lthn-desktop-controls-panel')).not.toBeNull();
    expect(fixture.nativeElement.textContent).toContain('Interface theme');
    expect(fixture.nativeElement.textContent).toContain('Restart required');
    expect(fixture.nativeElement.textContent).toContain('Previous save failed safely.');
    expect(fixture.nativeElement.textContent).toContain('Host: granted');
  });

  it('shows an opaque .lthn hand-off for explicit import review', async () => {
    hostIntents.claimItems.mockReturnValueOnce([
      {
        mountId: 'host-profile',
        path: 'profile.lthn',
        name: 'profile.lthn',
        kind: 'file',
        mediaType: 'application/x-lethean',
      },
    ]);
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', win);

    await fixture.whenStable();

    expect(hostIntents.onItems).toHaveBeenCalledWith('settings', expect.any(Function));
    expect(fixture.nativeElement.textContent).toContain('profile.lthn');
    expect(fixture.nativeElement.textContent).toContain('ready for import review');
  });

  it('requests native notification permission only after the explicit button is used', async () => {
    permissions.status.mockResolvedValueOnce([
      { id: 'microphone', policy: 'default', host: 'unsupported' },
      { id: 'camera', policy: 'default', host: 'unsupported' },
      { id: 'geolocation', policy: 'default', host: 'unsupported' },
      { id: 'notifications', policy: 'default', host: 'prompt' },
      { id: 'clipboard-read', policy: 'default', host: 'unsupported' },
    ]);
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', win);
    await fixture.whenStable();

    expect(permissions.request).not.toHaveBeenCalled();
    (
      fixture.nativeElement.querySelector(
        '[data-action="request-permission-notifications"]',
      ) as HTMLButtonElement
    ).click();
    await fixture.whenStable();

    expect(permissions.request).toHaveBeenCalledWith('notifications');
    expect(fixture.nativeElement.textContent).toContain('Host: granted');
  });

  it('edits only the draft when a control changes', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', win);
    await fixture.whenStable();

    const select = fixture.nativeElement.querySelector(
      '[data-control-key="desktop.theme.interface"] select',
    ) as HTMLSelectElement;
    select.value = 'light';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({
        key: 'desktop.theme.interface',
        value: 'light',
      }),
    );
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: desktopControlsActions.applyDraft.type }),
    );
  });

  it('applies, discards, and resets through explicit draft actions', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(SettingsApp);
    fixture.componentRef.setInput('win', win);
    await fixture.whenStable();

    (fixture.nativeElement.querySelector('[data-action="apply-settings"]') as HTMLElement).click();
    (
      fixture.nativeElement.querySelector('[data-action="discard-settings"]') as HTMLElement
    ).click();
    (fixture.nativeElement.querySelector('[data-action="reset-settings"]') as HTMLElement).click();
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.applyDraft({
        changes: [{ key: 'desktop.theme.interface', value: 'light' }],
      }),
    );
    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.discardDraft());
    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.resetDraft());
  });
});
