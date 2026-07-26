import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
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

describe('TelemetryApp', () => {
  const mode = signal<'demo' | 'live'>('demo');
  const liveData = {
    mode: mode.asReadonly(),
    telemetry: vi.fn(),
  };

  beforeEach(() => {
    mode.set('demo');
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [{ provide: DesktopLiveDataService, useValue: liveData }],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create() {
    const fixture = TestBed.createComponent(TelemetryApp);
    fixture.componentRef.setInput('win', telemetryWin);
    fixture.detectChanges();
    await fixture.whenStable();
    return fixture;
  }

  it('renders useful demo data without attempting a live read in offline mode', async () => {
    const fixture = await create();
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Demo data');
    expect(element.textContent).toContain('41.8');
    expect(element.textContent).toContain('llama-3.1-70b');
    expect(liveData.telemetry).not.toHaveBeenCalled();
  });

  it('renders live process metrics while labelling unsupported power data as demo', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue({
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
    });

    const fixture = await create();
    await vi.waitFor(() => expect(fixture.componentInstance.dataState()).toBe('mixed'));
    fixture.detectChanges();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Live + demo');
    expect(text).toContain('Heap allocation');
    expect(text).toContain('128.3');
    expect(text).toContain('Power draw · demo');
    expect(text).toContain('Goroutines 42');
    expect(text).toContain('GC pause 0.43 ms');
    expect(text).toContain('Uptime 2h 31m');
  });

  it('returns to labelled demo values when a later live sample fails', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValueOnce({
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
    });

    const fixture = await create();
    await vi.waitFor(() => expect(fixture.componentInstance.sample()).not.toBeNull());
    liveData.telemetry.mockRejectedValueOnce(new Error('telemetry unavailable'));

    await fixture.componentInstance.refresh();
    fixture.detectChanges();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(fixture.componentInstance.dataState()).toBe('unavailable');
    expect(fixture.componentInstance.sample()).toBeNull();
    expect(text).toContain('Live unavailable · demo shown');
    expect(text).toContain('Throughput');
    expect(text).toContain('41.8');
  });
});
