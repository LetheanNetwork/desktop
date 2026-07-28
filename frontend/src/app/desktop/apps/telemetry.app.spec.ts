// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { createDemoResource, type DesktopDataResource } from '../desktop-data-resource';
import type { ProcessTelemetry } from '../desktop-live-data.service';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { createDemoModelRuntimeSnapshot } from '../desktop-model-runtime-demo.data';
import type { ModelRuntimeOperation, ModelRuntimeSnapshot } from '../desktop-model-runtime.models';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
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
  const modelRuntimeState = signal<DesktopDataResource<ModelRuntimeSnapshot>>(
    createDemoResource(createDemoModelRuntimeSnapshot(), 'Lethean demo fixture · Model runtime'),
  );
  const modelRuntimePending = signal<ModelRuntimeOperation | null>(null);
  let modelRuntimeDisconnect = vi.fn();
  const modelRuntime = {
    resource: modelRuntimeState.asReadonly(),
    pending: modelRuntimePending.asReadonly(),
    connect: vi.fn(),
    perform: vi.fn<(operation: ModelRuntimeOperation) => Promise<void>>(),
  };

  beforeEach(() => {
    mode.set('demo');
    liveData.telemetry.mockReset();
    modelRuntimeDisconnect = vi.fn();
    modelRuntimeState.set(
      createDemoResource(createDemoModelRuntimeSnapshot(), 'Lethean demo fixture · Model runtime'),
    );
    modelRuntimePending.set(null);
    modelRuntime.connect.mockReset();
    modelRuntime.connect.mockReturnValue(modelRuntimeDisconnect);
    modelRuntime.perform.mockReset();
    TestBed.configureTestingModule({
      providers: [
        { provide: DesktopLiveDataService, useValue: liveData },
        { provide: DesktopModelRuntimeResource, useValue: modelRuntime },
      ],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    TestBed.resetTestingModule();
  });

  function create() {
    if (mode() === 'live' && modelRuntimeState().mode === 'demo') {
      setLiveModelRuntime(createDemoModelRuntimeSnapshot('model-less'));
    }
    const fixture = TestBed.createComponent(TelemetryApp);
    fixture.componentRef.setInput('win', telemetryWin);
    fixture.detectChanges();
    return fixture;
  }

  function setLiveModelRuntime(snapshot: ModelRuntimeSnapshot): void {
    modelRuntimeState.set({
      mode: 'connected',
      state: 'live',
      source: 'Local LEM runtime',
      updatedAt: Date.parse(snapshot.refreshedAt),
      refreshing: false,
      error: null,
      canRetry: false,
      value: snapshot,
    });
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
    expect(modelRuntime.connect).toHaveBeenCalledOnce();
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

  it('renders unsupported connected metrics as unavailable without fixture substitution', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue(SAMPLE);
    const fixture = create();

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent;

    expect(text).toContain('Live data');
    expect(text).toContain('Local process runtime');
    expect(text).toContain('Throughput');
    expect(text).toContain('Power draw');
    expect(text).toContain('Model —');
    expect(text).toContain('Memory —');
    expect(text).toContain('Uptime 2h 31m');
    expect(text).not.toContain('41.8');
    expect(text).not.toContain('207');
    expect(fixture.componentInstance.resource().updatedAt).not.toBeNull();
  });

  it('reacts to the same shared runtime snapshot without another process read', async () => {
    mode.set('live');
    setLiveModelRuntime(createDemoModelRuntimeSnapshot('model-less'));
    liveData.telemetry.mockResolvedValue(SAMPLE);
    const fixture = create();
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
    liveData.telemetry.mockClear();
    const ready = createDemoModelRuntimeSnapshot('ready');

    setLiveModelRuntime(ready);
    fixture.detectChanges();

    expect(fixture.componentInstance.modelRuntimeResource().value).toBe(ready);
    expect(fixture.nativeElement.textContent).toContain('gemma-4-e2b');
    expect(fixture.nativeElement.textContent).toContain('41.8');
    expect(liveData.telemetry).not.toHaveBeenCalled();
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
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
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
    expect(fixture.nativeElement.textContent).toContain('Throughput');
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

    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
    expect(liveData.telemetry).toHaveBeenCalledTimes(2);
  });

  it('recovers stale data without resetting its successful history', async () => {
    mode.set('live');
    liveData.telemetry
      .mockResolvedValueOnce({ ...SAMPLE, wattsActive: 200 })
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ ...SAMPLE, wattsActive: 230 });
    const fixture = create();
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));

    await fixture.componentInstance.refresh();
    expect(fixture.componentInstance.resource().state).toBe('stale');
    await fixture.componentInstance.refresh();

    expect(fixture.componentInstance.resource()).toMatchObject({
      state: 'live',
      error: null,
      canRetry: false,
    });
    expect(fixture.componentInstance.resource().value?.power.history).toEqual([200, 230]);
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
    await vi.waitFor(() => expect(fixture.componentInstance.resource().state).toBe('live'));
  });

  it('polls every five seconds and stops polling when destroyed', async () => {
    vi.useFakeTimers();
    mode.set('live');
    liveData.telemetry.mockResolvedValue(SAMPLE);
    const fixture = create();

    await vi.advanceTimersByTimeAsync(0);
    expect(liveData.telemetry).toHaveBeenCalledOnce();

    liveData.telemetry.mockClear();
    await vi.advanceTimersByTimeAsync(4_999);
    expect(liveData.telemetry).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(liveData.telemetry).toHaveBeenCalledOnce();

    fixture.destroy();
    liveData.telemetry.mockClear();
    await vi.advanceTimersByTimeAsync(5_000);
    expect(liveData.telemetry).not.toHaveBeenCalled();
  });

  it('does not create a polling timer in demo mode', () => {
    vi.useFakeTimers();
    const setInterval = vi.spyOn(window, 'setInterval');
    const fixture = create();

    vi.advanceTimersByTime(15_000);

    expect(setInterval).not.toHaveBeenCalled();
    expect(liveData.telemetry).not.toHaveBeenCalled();
    fixture.destroy();
  });

  it('disconnects its shared runtime consumer when the Telemetry window closes', () => {
    const fixture = create();

    fixture.destroy();

    expect(modelRuntimeDisconnect).toHaveBeenCalledOnce();
  });

  it('ignores a live result that settles after destruction', async () => {
    mode.set('live');
    let resolve!: (sample: ProcessTelemetry) => void;
    liveData.telemetry.mockReturnValue(
      new Promise<ProcessTelemetry>((accept) => {
        resolve = accept;
      }),
    );
    const fixture = create();
    const resourceAtDestroy = fixture.componentInstance.resource();

    fixture.destroy();
    resolve(SAMPLE);
    await Promise.resolve();
    await Promise.resolve();

    expect(fixture.componentInstance.resource()).toBe(resourceAtDestroy);
    expect(fixture.componentInstance.resource().value).toBeNull();
  });
});
