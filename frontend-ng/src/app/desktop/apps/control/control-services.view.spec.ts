// SPDX-License-Identifier: EUPL-1.2

import { TestBed } from '@angular/core/testing';
import {
  createConnectedResource,
  createDemoResource,
  type DesktopDataResource,
} from '../../desktop-data-resource';
import {
  createDemoServiceCatalogue,
  SERVICES_DEMO_SOURCE,
  type DesktopServiceCatalogue,
  type DesktopServiceOutput,
} from './control-services.models';
import { ControlServicesView } from './control-services.view';

function connectedResource(
  state: 'live' | 'stale' = 'live',
): DesktopDataResource<DesktopServiceCatalogue> {
  return {
    ...createConnectedResource<DesktopServiceCatalogue>('Local go-process runtime'),
    state,
    updatedAt: Date.parse('2026-07-27T12:00:00Z'),
    error: state === 'stale' ? 'Refresh failed.' : null,
    canRetry: state === 'stale',
    value: createDemoServiceCatalogue(),
  };
}

function outputFixture(): DesktopServiceOutput {
  return {
    id: 'api',
    processId: 'proc-1',
    generation: 1,
    output: 'ready\n',
    truncated: false,
    observedAt: '2026-07-27T12:00:00Z',
  };
}

async function createFixture(
  resource: DesktopDataResource<DesktopServiceCatalogue>,
  pendingIds: readonly string[] = [],
  output: DesktopServiceOutput | null = null,
) {
  const fixture = TestBed.createComponent(ControlServicesView);
  fixture.componentRef.setInput('resource', resource);
  fixture.componentRef.setInput('pendingIds', pendingIds);
  fixture.componentRef.setInput('outputView', output);
  await fixture.whenStable();
  return fixture;
}

describe('ControlServicesView', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('renders manual service controls from a visibly labelled demo resource', async () => {
    const fixture = await createFixture(
      createDemoResource(createDemoServiceCatalogue(), SERVICES_DEMO_SOURCE),
    );
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Services');
    expect(element.textContent).toContain('Starts manually');
    expect(element.textContent).toContain(SERVICES_DEMO_SOURCE);
    expect(element.querySelector('[data-service-id="api"] [data-action="stop"]')).not.toBeNull();
    expect(
      element.querySelector('[data-service-id="runner"] [data-action="start"]'),
    ).not.toBeNull();
    expect(element.textContent).not.toContain('workingDirectory');
    expect(element.textContent).not.toContain('arguments');
  });

  it('emits a typed restart intent and disables duplicate actions while pending', async () => {
    const fixture = await createFixture(connectedResource(), ['api']);
    const emitted = vi.fn();
    fixture.componentInstance.action.subscribe(emitted);
    const element = fixture.nativeElement as HTMLElement;
    const restart = element.querySelector<HTMLButtonElement>(
      '[data-service-id="api"] [data-action="restart"]',
    );

    expect(restart?.disabled).toBe(true);
    expect(element.querySelector('[data-service-id="api"]')?.textContent).toContain('Working');

    fixture.componentRef.setInput('pendingIds', []);
    await fixture.whenStable();
    element
      .querySelector<HTMLButtonElement>('[data-service-id="api"] [data-action="restart"]')
      ?.click();
    expect(emitted).toHaveBeenCalledWith({ kind: 'restart', id: 'api' });
  });

  it('shows stale data and bounded output only after it is supplied', async () => {
    const fixture = await createFixture(connectedResource('stale'), [], outputFixture());
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Live data stale');
    expect(element.querySelector('pre')?.textContent).toContain('ready');
    expect(element.textContent).toContain('api output');
  });

  it('emits Retry for a stale resource and renders an unavailable empty state', async () => {
    const resource: DesktopDataResource<DesktopServiceCatalogue> = {
      ...createConnectedResource<DesktopServiceCatalogue>('Local go-process runtime'),
      state: 'unavailable',
      error: 'Live Services data is unavailable.',
      canRetry: true,
    };
    const fixture = await createFixture(resource);
    const retry = vi.fn();
    fixture.componentInstance.retry.subscribe(retry);

    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('[data-action="retry"]')
      ?.click();

    expect(retry).toHaveBeenCalledOnce();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'No services are available',
    );
  });

  it('keeps force actions off the main row, behind an overflow', async () => {
    const fixture = await createFixture(connectedResource());
    const overflow = fixture.nativeElement.querySelector('details.service__force');

    // The unusual responses must not sit beside Stop at equal weight.
    expect(overflow).not.toBeNull();
    expect(
      fixture.nativeElement.querySelector('.service__actions > [data-action="kill"]'),
    ).toBeNull();
    expect(overflow.querySelector('[data-action="kill"]')).not.toBeNull();
    expect(overflow.querySelector('[data-action="signal-hangup"]')).not.toBeNull();
  });

  it('emits a named signal intent, never a number', async () => {
    const fixture = await createFixture(connectedResource());
    const emitted: unknown[] = [];
    fixture.componentInstance.action.subscribe((intent: unknown) => emitted.push(intent));

    fixture.nativeElement.querySelector('[data-action="signal-hangup"]').click();
    fixture.nativeElement.querySelector('[data-action="kill"]').click();
    await fixture.whenStable();

    expect(emitted).toEqual([
      expect.objectContaining({ kind: 'signal', signal: 'hangup' }),
      expect.objectContaining({ kind: 'kill' }),
    ]);
  });
});
