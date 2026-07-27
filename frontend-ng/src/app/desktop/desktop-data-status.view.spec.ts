// SPDX-License-Identifier: EUPL-1.2

import { TestBed } from '@angular/core/testing';
import type { DesktopDataStatus } from './desktop-data-resource';
import { DesktopDataStatusView } from './desktop-data-status.view';

describe('DesktopDataStatusView', () => {
  const live: DesktopDataStatus = {
    mode: 'connected',
    state: 'live',
    source: 'Local process runtime',
    updatedAt: new Date(2026, 6, 27, 12, 34, 56).getTime(),
    refreshing: false,
    error: null,
    canRetry: false,
  };

  function create(status: DesktopDataStatus) {
    const fixture = TestBed.createComponent(DesktopDataStatusView);
    fixture.componentRef.setInput('status', status);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => TestBed.resetTestingModule());

  it.each([
    ['demo', 'Demo data', 'warn'],
    ['loading', 'Loading live data', 'muted'],
    ['live', 'Live data', 'ok'],
    ['mixed', 'Live + demo', 'warn'],
    ['stale', 'Live data stale', 'warn'],
    ['unavailable', 'Live unavailable', 'warn'],
  ] as const)('renders the %s state label and established variant', (state, label, variant) => {
    const fixture = create({
      ...live,
      mode: state === 'demo' ? 'demo' : 'connected',
      state,
      updatedAt: null,
    });
    const badge = fixture.nativeElement.querySelector('lthn-badge');

    expect(badge.textContent).toContain(label);
    expect(badge.getAttribute('variant')).toBe(variant);
    expect(badge.dataset['dataState']).toBe(state);
  });

  it('shows reader-facing source and local receipt time', () => {
    const text = create(live).nativeElement.textContent;

    expect(text).toContain('Local process runtime');
    expect(text).toContain('Received 12:34:56');
  });

  it('shows refreshing without hiding the current live status', () => {
    const text = create({ ...live, refreshing: true }).nativeElement.textContent;

    expect(text).toContain('Live data');
    expect(text).toContain('Refreshing');
  });

  it('shows stale and unavailable explanations', () => {
    expect(
      create({
        ...live,
        state: 'stale',
        error: null,
      }).nativeElement.textContent,
    ).toContain('The last live reading is being retained.');
    expect(
      create({
        ...live,
        state: 'unavailable',
        updatedAt: null,
        error: 'Live telemetry is unavailable.',
      }).nativeElement.textContent,
    ).toContain('Live telemetry is unavailable.');
  });

  it('emits Retry only for a completed connected failure', () => {
    const fixture = create({
      ...live,
      state: 'stale',
      error: 'Refresh failed.',
      canRetry: true,
    });
    const emitted = vi.fn();
    fixture.componentInstance.retry.subscribe(emitted);
    const retry = fixture.nativeElement.querySelector('button[data-action="retry"]');

    retry.click();

    expect(retry.getAttribute('aria-label')).toBe('Retry live data');
    expect(emitted).toHaveBeenCalledOnce();
  });

  it.each([
    { ...live, mode: 'demo' as const, state: 'demo' as const, canRetry: true },
    { ...live, state: 'stale' as const, canRetry: true, error: null },
    {
      ...live,
      state: 'stale' as const,
      canRetry: true,
      error: 'Refresh failed.',
      refreshing: true,
    },
  ])('does not expose Retry for invalid presentation state %#', (status) => {
    expect(create(status).nativeElement.querySelector('button')).toBeNull();
  });
});
