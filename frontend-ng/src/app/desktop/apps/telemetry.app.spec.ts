// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import type { ProcessTelemetry } from '../desktop-live-data.service';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import type { Win } from '../desktop.data';
import { TelemetryApp } from './telemetry.app';

const telemetryWin: Win = {
  id: 'telemetry-window',
  app: 'telemetry',
  sub: '',
  x: 0,
  y: 0,
  w: 620,
  h: 420,
  z: 1,
  min: false,
  max: false,
};

const SAMPLE: ProcessTelemetry = {
  heapAllocMB: 128.25,
  heapSysMB: 192.5,
  stackInUseMB: 4.75,
  numGoroutines: 42,
  numCgoCalls: 7,
  uptimeSeconds: 9_061,
  numGC: 18,
  lastGCPauseMs: 0.43,
  wattsActive: 0,
  wattsIdle: 0,
};

describe('TelemetryApp', () => {
  const mode = signal<'demo' | 'live'>('demo');
  const liveData = {
    mode: mode.asReadonly(),
    telemetry: vi.fn<() => Promise<ProcessTelemetry>>(),
  };

  beforeEach(() => {
    mode.set('demo');
    liveData.telemetry.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: DesktopLiveDataService, useValue: liveData }],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    TestBed.resetTestingModule();
  });

  function create() {
    const fixture = TestBed.createComponent(TelemetryApp);
    fixture.componentRef.setInput('win', telemetryWin);
    fixture.detectChanges();
    return fixture;
  }

  it('renders deterministic labelled demo data without a live read', () => {
    const fixture = create();
    const text = fixture.nativeElement.textContent;

    expect(fixture.componentInstance.resource().state).toBe('demo');
    expect(text).toContain('Demo data');
    expect(text).toContain('Lethean demo fixture');
    expect(text).toContain('41.8');
    expect(text).toContain('llama-3.1-70b');
    expect(liveData.telemetry).not.toHaveBeenCalled();
  });

  it('starts connected with loading placeholders and no fixture substitution', () => {
    mode.set('live');
    liveData.telemetry.mockReturnValue(new Promise(() => undefined));
    const fixture = create();
    const text = fixture.nativeElement.textContent;

    expect(fixture.componentInstance.resource()).toMatchObject({
      mode: 'connected',
      state: 'loading',
      refreshing: true,
      value: null,
    });
    expect(text).toContain('Loading live data');
    expect(text).not.toContain('41.8');
    expect(text).not.toContain('207');
    expect(text).not.toContain('llama-3.1-70b');
  });

  it('renders live process data with explicitly demo-backed power', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue(SAMPLE);
    const fixture = create();

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('mixed'));
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent;

    expect(text).toContain('Live + demo');
    expect(text).toContain('Local process runtime');
    expect(text).toContain('Heap allocation');
    expect(text).toContain('128.3');
    expect(text).toContain('Power draw · demo');
    expect(text).toContain('Goroutines 42');
    expect(text).toContain('Uptime 2h 31m');
    expect(fixture.componentInstance.resource().updatedAt).not.toBeNull();
  });

  it('renders positive native power as wholly live', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue({ ...SAMPLE, wattsActive: 220.4 });
    const fixture = create();

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Live data');
    expect(fixture.nativeElement.textContent).toContain('220');
    expect(fixture.nativeElement.textContent).not.toContain('Power draw · demo');
  });

  it('makes a first failure unavailable without showing fixtures or raw errors', async () => {
    mode.set('live');
    liveData.telemetry.mockRejectedValue(new Error('socket secret detail'));
    const fixture = create();

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('unavailable'));
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent;

    expect(text).toContain('Live telemetry is unavailable.');
    expect(text).not.toContain('socket secret detail');
    expect(text).not.toContain('41.8');
    expect(text).not.toContain('207');
    expect(fixture.componentInstance.resource().value).toBeNull();
  });

  it('retains the last successful sample and charts as stale after a later failure', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValueOnce(SAMPLE);
    const fixture = create();
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('mixed'));
    const successful = fixture.componentInstance.resource();

    liveData.telemetry.mockRejectedValueOnce(new Error('connection dropped'));
    await fixture.componentInstance.refresh();
    fixture.detectChanges();

    expect(fixture.componentInstance.resource()).toMatchObject({
      state: 'stale',
      updatedAt: successful.updatedAt,
      value: successful.value,
      canRetry: true,
    });
    expect(fixture.nativeElement.textContent).toContain('128.3');
    expect(fixture.nativeElement.textContent).toContain('Live telemetry is unavailable.');
  });

  it('routes the status Retry action through the same refresh path', async () => {
    mode.set('live');
    liveData.telemetry.mockRejectedValueOnce(new Error('offline'));
    const fixture = create();
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('unavailable'));
    liveData.telemetry.mockResolvedValueOnce(SAMPLE);
    fixture.detectChanges();

    fixture.nativeElement.querySelector('button[data-action="retry"]').click();

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('mixed'));
    expect(liveData.telemetry).toHaveBeenCalledTimes(2);
  });

  it('recovers stale data without resetting its successful history', async () => {
    mode.set('live');
    liveData.telemetry
      .mockResolvedValueOnce(SAMPLE)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ ...SAMPLE, heapAllocMB: 130 });
    const fixture = create();
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('mixed'));

    await fixture.componentInstance.refresh();
    expect(fixture.componentInstance.resource().state).toBe('stale');
    await fixture.componentInstance.refresh();

    expect(fixture.componentInstance.resource()).toMatchObject({
      state: 'mixed',
      error: null,
      canRetry: false,
    });
    expect(fixture.componentInstance.resource().value?.primary.history).toEqual([128.25, 130]);
  });

  it('skips an overlapping manual refresh', async () => {
    mode.set('live');
    let resolve!: (sample: ProcessTelemetry) => void;
    liveData.telemetry.mockReturnValue(
      new Promise<ProcessTelemetry>((accept) => {
        resolve = accept;
      }),
    );
    const fixture = create();

    void fixture.componentInstance.refresh();
    expect(liveData.telemetry).toHaveBeenCalledOnce();

    resolve(SAMPLE);
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('mixed'));
  });
});
