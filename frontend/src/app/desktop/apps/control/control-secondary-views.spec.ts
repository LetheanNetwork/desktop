import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Store } from '@ngrx/store';
import { provideMockStore } from '@ngrx/store/testing';
import { desktopControlsActions } from '../../../store/desktop-controls.actions';
import type { DesktopControl, DesktopControlGroup } from '../../../store/desktop-controls.models';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectHasDirtyDesktopControls,
} from '../../../store/desktop-controls.reducer';
import { ConnectionManagerService } from '../../../connection-manager.service';
import { createDemoResource } from '../../desktop-data-resource';
import {
  SYSTEM_MONITOR_DEMO_SNAPSHOT,
  SYSTEM_MONITOR_DEMO_SOURCE,
} from '../../desktop-system-monitor-demo.data';
import { ControlPowerView } from './control-power.view';
import { createDemoServiceCatalogue, SERVICES_DEMO_SOURCE } from './control-services.models';
import { ControlSettingsView } from './control-settings.view';
import { ControlSystemView } from './control-system.view';
import { createDemoControlViewState } from './control-view-state';

describe('Control secondary views', () => {
  const widthControl: DesktopControl = {
    key: 'desktop.wails.window.main.width',
    group: 'Window',
    label: 'Window width',
    description: 'Width of the main desktop window in pixels.',
    kind: 'number',
    value: 1_440,
    defaultValue: 1_440,
    configured: false,
    live: true,
    restartRequired: false,
    minimum: 800,
    maximum: 3_840,
    step: 10,
  };
  const groups: readonly DesktopControlGroup[] = [{ name: 'Window', controls: [widthControl] }];

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConnectionManagerService,
          useValue: { offline: signal(false).asReadonly() },
        },
        provideMockStore({
          selectors: [
            { selector: selectDesktopControlGroups, value: groups },
            {
              selector: selectDirtyDesktopControlChanges,
              value: [{ key: widthControl.key, value: 1_280 }],
            },
            { selector: selectHasDirtyDesktopControls, value: true },
            { selector: desktopControlsFeature.selectLoading, value: false },
            { selector: desktopControlsFeature.selectSaving, value: false },
            { selector: desktopControlsFeature.selectStale, value: false },
            { selector: desktopControlsFeature.selectPendingExternalChange, value: null },
            { selector: desktopControlsFeature.selectError, value: null },
            { selector: desktopControlsFeature.selectRestartSummary, value: null },
          ],
        }),
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  it('keeps the complete Power prototype', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlPowerView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.power);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    const text = element.textContent ?? '';
    expect(text).toContain('Power');
    expect(text).toContain('≈ a small fridge');
    expect(element.querySelectorAll('lthn-stat')).toHaveLength(3);
    expect(element.querySelector('lthn-chart')?.getAttribute('data')).toBe(
      '[180,176,182,190,188,195,201,198,205,199,207,210]',
    );
  });

  it('emits a typed System tab request', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlSystemView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.system);
    fixture.componentRef.setInput('activeTab', 'overview');
    fixture.componentRef.setInput(
      'systemResource',
      createDemoResource(SYSTEM_MONITOR_DEMO_SNAPSHOT, SYSTEM_MONITOR_DEMO_SOURCE),
    );
    fixture.componentRef.setInput(
      'services',
      createDemoResource(createDemoServiceCatalogue(), SERVICES_DEMO_SOURCE),
    );
    const emitted = vi.fn();
    fixture.componentInstance.tabChange.subscribe(emitted);

    await fixture.whenStable();

    const buttons = (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLButtonElement>(
      'button.systab',
    );
    expect(buttons).toHaveLength(3);
    buttons[1].click();
    expect(emitted).toHaveBeenCalledWith('processes');
  });

  it('keeps the daemons state value while presenting the working Services view', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlSystemView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.system);
    fixture.componentRef.setInput('activeTab', 'daemons');
    fixture.componentRef.setInput(
      'systemResource',
      createDemoResource(SYSTEM_MONITOR_DEMO_SNAPSHOT, SYSTEM_MONITOR_DEMO_SOURCE),
    );
    fixture.componentRef.setInput(
      'services',
      createDemoResource(createDemoServiceCatalogue(), SERVICES_DEMO_SOURCE),
    );

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    const buttons = element.querySelectorAll<HTMLButtonElement>('button.systab');
    expect(buttons[2].textContent?.trim()).toBe('Services');
    expect(element.querySelector('lthn-control-services-view')).not.toBeNull();
    expect(element.textContent).toContain('Lethean API');
  });

  it('renders and commits the shared NgRx desktop-control draft', async () => {
    const store = TestBed.inject(Store);
    const dispatch = vi.spyOn(store, 'dispatch');
    const fixture = TestBed.createComponent(ControlSettingsView);
    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Configuration');
    expect(element.textContent).toContain('Defaults → File → Env → Set');
    expect(element.textContent).toContain('CORE_CONFIG_*');
    expect(element.textContent).toContain('Window width');
    const input = element.querySelector<HTMLInputElement>(
      '[data-control-key="desktop.wails.window.main.width"] input',
    );
    if (!input) throw new Error('Expected the shared Window width control.');
    input.value = '1280';
    input.dispatchEvent(new Event('change', { bubbles: true }));
    element.querySelector<HTMLButtonElement>('[data-action="apply-settings"]')?.click();
    await fixture.whenStable();

    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.editControl({ key: widthControl.key, value: 1_280 }),
    );
    expect(dispatch).toHaveBeenCalledWith(
      desktopControlsActions.applyDraft({
        changes: [{ key: widthControl.key, value: 1_280 }],
      }),
    );
  });
});
