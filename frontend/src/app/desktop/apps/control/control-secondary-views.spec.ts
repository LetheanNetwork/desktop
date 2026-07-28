import { TestBed } from '@angular/core/testing';
import { createDemoResource } from '../../desktop-data-resource';
import { ControlPowerView } from './control-power.view';
import { createDemoServiceCatalogue, SERVICES_DEMO_SOURCE } from './control-services.models';
import { ControlSettingsView } from './control-settings.view';
import { ControlSystemView } from './control-system.view';
import { createDemoControlViewState } from './control-view-state';

describe('Control secondary views', () => {
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

  it('renders settings and emits Commit', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlSettingsView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.settings);
    const emitted = vi.fn();
    fixture.componentInstance.commit.subscribe(emitted);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('features.lethernet');
    expect(element.querySelector('lthn-toggle[on]')).not.toBeNull();
    element.querySelector<HTMLButtonElement>('button.nbtn')?.click();
    expect(emitted).toHaveBeenCalledOnce();
  });
});
