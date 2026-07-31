// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideMockStore } from '@ngrx/store/testing';
import { Store } from '@ngrx/store';
import { desktopControlsActions } from '../store/desktop-controls.actions';
import type { DesktopControl, DesktopControlGroup } from '../store/desktop-controls.models';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectHasDirtyDesktopControls,
} from '../store/desktop-controls.reducer';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlsPanelView } from './desktop-controls-panel.view';

const controls: readonly DesktopControl[] = [
  {
    key: 'desktop.shell.show_widgets',
    group: 'Desktop',
    label: 'Desktop widgets',
    description: 'Show widgets on the desktop.',
    kind: 'toggle',
    value: true,
    defaultValue: true,
    configured: false,
    live: true,
    restartRequired: false,
  },
  {
    key: 'desktop.theme.interface',
    group: 'Theme',
    label: 'Interface theme',
    description: 'Choose the interface theme.',
    kind: 'select',
    value: 'dark',
    defaultValue: 'dark',
    configured: false,
    live: true,
    restartRequired: false,
    choices: ['dark', 'light'],
  },
  {
    key: 'desktop.wails.window.main.width',
    group: 'Window',
    label: 'Window width',
    description: 'Set the main window width.',
    kind: 'number',
    value: 1_440,
    defaultValue: 1_440,
    configured: false,
    live: true,
    restartRequired: false,
    minimum: 800,
    maximum: 3_840,
    step: 10,
  },
  {
    key: 'desktop.wails.application.name',
    group: 'Application',
    label: 'Application name',
    description: 'Set the native application name.',
    kind: 'text',
    value: 'Lethean',
    defaultValue: 'Lethean',
    configured: true,
    live: false,
    restartRequired: true,
  },
  {
    key: 'desktop.permissions.notifications',
    group: 'Permissions',
    label: 'Notifications',
    description: 'Allow native notifications.',
    kind: 'toggle',
    value: false,
    defaultValue: false,
    configured: false,
    live: true,
    restartRequired: false,
  },
];

const groups: readonly DesktopControlGroup[] = [
  { name: 'Desktop', controls: [controls[0]] },
  { name: 'Theme', controls: [controls[1]] },
  { name: 'Window', controls: [controls[2]] },
  { name: 'Application', controls: [controls[3]] },
  { name: 'Permissions', controls: [controls[4]] },
];

describe('DesktopControlsPanelView', () => {
  const offline = signal(false);

  beforeEach(() => {
    offline.set(false);
    TestBed.configureTestingModule({
      imports: [DesktopControlsPanelView],
      providers: [
        { provide: ConnectionManagerService, useValue: { offline: offline.asReadonly() } },
        provideMockStore({
          selectors: [
            { selector: selectDesktopControlGroups, value: groups },
            {
              selector: selectDirtyDesktopControlChanges,
              value: [
                { key: 'desktop.theme.interface', value: 'light' },
                { key: 'desktop.wails.window.main.width', value: 1_280 },
              ],
            },
            { selector: selectHasDirtyDesktopControls, value: true },
            { selector: desktopControlsFeature.selectLoading, value: false },
            { selector: desktopControlsFeature.selectSaving, value: false },
            {
              selector: desktopControlsFeature.selectError,
              value: 'The previous save failed safely.',
            },
            {
              selector: desktopControlsFeature.selectRestartSummary,
              value: 'Restart required for: Application name.',
            },
          ],
        }),
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  it('renders grouped selector state while excluding keys owned by a curated surface', async () => {
    const fixture = TestBed.createComponent(DesktopControlsPanelView);
    fixture.componentRef.setInput('excludedKeys', ['desktop.shell.show_widgets']);
    fixture.componentRef.setInput('permissions', [
      { id: 'notifications', policy: 'default', host: 'granted' },
    ]);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Interface theme');
    expect(element.textContent).not.toContain('Desktop widgets');
    expect(element.textContent).toContain('The previous save failed safely.');
    expect(element.textContent).toContain('Restart required for: Application name.');
    expect(element.textContent).toContain('Host: granted');
  });

  it('retains configuration context and visibly labels the offline demo store', async () => {
    offline.set(true);
    const fixture = TestBed.createComponent(DesktopControlsPanelView);
    fixture.componentRef.setInput('precedence', 'Defaults → File → Env → Set');
    fixture.componentRef.setInput(
      'help',
      'Env overrides use CORE_CONFIG_*; environment values are never written back on Apply.',
    );

    await fixture.whenStable();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Demo data');
    expect(text).toContain('Defaults → File → Env → Set');
    expect(text).toContain('desktop.wails.window.main.width');
    expect(text).toContain('CORE_CONFIG_*');
  });

  it('dispatches typed edits for every supported control kind', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(DesktopControlsPanelView);
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;

    element
      .querySelector<HTMLButtonElement>(
        '[data-control-key="desktop.shell.show_widgets"] [data-value="false"]',
      )
      ?.click();
    const select = element.querySelector<HTMLSelectElement>(
      '[data-control-key="desktop.theme.interface"] select',
    );
    if (!select) throw new Error('Expected the interface theme selector.');
    select.value = 'light';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    const number = element.querySelector<HTMLInputElement>(
      '[data-control-key="desktop.wails.window.main.width"] input',
    );
    if (!number) throw new Error('Expected the window width input.');
    number.value = '1280';
    number.dispatchEvent(new Event('change', { bubbles: true }));
    const text = element.querySelector<HTMLInputElement>(
      '[data-control-key="desktop.wails.application.name"] input',
    );
    if (!text) throw new Error('Expected the application name input.');
    text.value = 'Lethean Desktop';
    text.dispatchEvent(new Event('change', { bubbles: true }));
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({
        key: 'desktop.shell.show_widgets',
        value: false,
      }),
    );
    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({ key: 'desktop.theme.interface', value: 'light' }),
    );
    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({
        key: 'desktop.wails.window.main.width',
        value: 1_280,
      }),
    );
    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({
        key: 'desktop.wails.application.name',
        value: 'Lethean Desktop',
      }),
    );
  });

  it('keeps persistence explicit through NgRx draft actions', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(DesktopControlsPanelView);
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;

    element.querySelector<HTMLButtonElement>('[data-action="apply-settings"]')?.click();
    element.querySelector<HTMLButtonElement>('[data-action="discard-settings"]')?.click();
    element.querySelector<HTMLButtonElement>('[data-action="reset-settings"]')?.click();
    element.querySelector<HTMLButtonElement>('[data-action="retry-settings"]')?.click();
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.applyDraft({
        changes: [
          { key: 'desktop.theme.interface', value: 'light' },
          { key: 'desktop.wails.window.main.width', value: 1_280 },
        ],
      }),
    );
    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.discardDraft());
    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.resetDraft());
    expect(dispatch).toHaveBeenCalledWith(desktopControlsActions.load());
  });

  it('emits a native permission request only after the explicit action', async () => {
    const fixture = TestBed.createComponent(DesktopControlsPanelView);
    fixture.componentRef.setInput('permissions', [
      { id: 'notifications', policy: 'default', host: 'prompt' },
    ]);
    const emitted = vi.fn();
    fixture.componentInstance.permissionRequest.subscribe(emitted);
    await fixture.whenStable();

    expect(emitted).not.toHaveBeenCalled();
    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('[data-action="request-permission-notifications"]')
      ?.click();

    expect(emitted).toHaveBeenCalledWith('notifications');
  });
});
