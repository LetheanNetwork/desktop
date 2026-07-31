// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from '../desktop-data-resource';
import { createDemoModelRuntimeSnapshot } from '../desktop-model-runtime-demo.data';
import type { ModelRuntimeSnapshot } from '../desktop-model-runtime.models';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
import {
  SYSTEM_MONITOR_DEMO_SNAPSHOT,
  SYSTEM_MONITOR_DEMO_SOURCE,
} from '../desktop-system-monitor-demo.data';
import type { SystemMonitorSnapshot } from '../desktop-system-monitor.models';
import { DesktopSystemMonitorResource } from '../desktop-system-monitor-resource.service';
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
  const systemState = signal<DesktopDataResource<SystemMonitorSnapshot>>(
    createDemoResource(SYSTEM_MONITOR_DEMO_SNAPSHOT, SYSTEM_MONITOR_DEMO_SOURCE),
  );
  let systemDisconnect = vi.fn();
  const systemMonitor = {
    resource: systemState.asReadonly(),
    connect: vi.fn(),
    refresh: vi.fn<() => Promise<void>>(),
  };
  const modelState = signal<DesktopDataResource<ModelRuntimeSnapshot>>(
    createDemoResource(createDemoModelRuntimeSnapshot('ready'), 'Lethean demo fixture'),
  );
  let modelDisconnect = vi.fn();
  const modelRuntime = {
    resource: modelState.asReadonly(),
    connect: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    systemDisconnect = vi.fn();
    modelDisconnect = vi.fn();
    systemState.set(createDemoResource(SYSTEM_MONITOR_DEMO_SNAPSHOT, SYSTEM_MONITOR_DEMO_SOURCE));
    modelState.set(
      createDemoResource(createDemoModelRuntimeSnapshot('ready'), 'Lethean demo fixture'),
    );
    systemMonitor.connect.mockReturnValue(systemDisconnect);
    systemMonitor.refresh.mockResolvedValue();
    modelRuntime.connect.mockReturnValue(modelDisconnect);
    TestBed.configureTestingModule({
      providers: [
        { provide: DesktopSystemMonitorResource, useValue: systemMonitor },
        { provide: DesktopModelRuntimeResource, useValue: modelRuntime },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  function create() {
    const fixture = TestBed.createComponent(TelemetryApp);
    fixture.componentRef.setInput('win', telemetryWin);
    return fixture;
  }

  it('renders deterministic labelled host demo data from the shared resource', async () => {
    const fixture = create();
    await fixture.whenStable();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain(SYSTEM_MONITOR_DEMO_SOURCE);
    expect(text).toContain('CPU utilisation');
    expect(text).toContain('34');
    expect(text).toContain('Memory used');
    expect(text).toContain('58');
    expect(text).toContain('gemma-4-e2b');
    expect(systemMonitor.connect).toHaveBeenCalledOnce();
    expect(modelRuntime.connect).toHaveBeenCalledOnce();
  });

  it('shows loading placeholders without substituting host or AI fixtures', async () => {
    systemState.set(createConnectedResource<SystemMonitorSnapshot>('Local host system'));
    modelState.set(createConnectedResource<ModelRuntimeSnapshot>('Local model runtime'));
    const fixture = create();
    await fixture.whenStable();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Loading live data');
    expect(text).not.toContain('34');
    expect(text).not.toContain('58');
    expect(text).not.toContain('gemma-4-e2b');
  });

  it('reacts to the same live host and model snapshots used by Control', async () => {
    const host: SystemMonitorSnapshot = {
      ...SYSTEM_MONITOR_DEMO_SNAPSHOT,
      source: 'macOS host APIs',
      cpu: { logicalCores: 10, usagePercent: 37.5 },
      memory: { totalBytes: 32 * 1_024 ** 3, usedBytes: 16 * 1_024 ** 3 },
      cpuHistory: [30, 35, 37.5],
      memoryHistory: [48, 49, 50],
    };
    const hostLoading = createConnectedResource<SystemMonitorSnapshot>('Local host system');
    systemState.set(
      resolveDesktopData(
        beginDesktopDataRefresh(hostLoading, Date.now(), 10_000),
        host,
        'live',
        host.source,
        Date.parse(host.observedAt),
      ),
    );
    const runtime = createDemoModelRuntimeSnapshot('ready');
    const modelLoading = createConnectedResource<ModelRuntimeSnapshot>('Local model runtime');
    modelState.set(
      resolveDesktopData(
        beginDesktopDataRefresh(modelLoading, Date.now(), 10_000),
        runtime,
        'live',
        'Local model runtime',
        Date.now(),
      ),
    );

    const fixture = create();
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Live data');
    expect(element.textContent).toContain('macOS host APIs');
    expect(element.textContent).toContain('CPU utilisation');
    expect(element.textContent).toContain('38');
    expect(element.textContent).toContain('Memory used');
    expect(element.textContent).toContain('50');
    expect(element.textContent).toContain('gemma-4-e2b');
    expect(element.querySelectorAll('lthn-sparkline')[0].getAttribute('data')).toBe('[30,35,37.5]');
  });

  it('routes stale-data Retry through the shared resource', async () => {
    const loading = beginDesktopDataRefresh(
      createConnectedResource<SystemMonitorSnapshot>('Local host system'),
      Date.now(),
      10_000,
    );
    systemState.set(rejectDesktopData(loading, 'Live system information is unavailable.'));
    const fixture = create();
    await fixture.whenStable();

    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('button[data-action="retry"]')
      ?.click();
    await fixture.whenStable();

    expect(systemMonitor.refresh).toHaveBeenCalledOnce();
  });

  it('does not install its own polling timer', () => {
    vi.useFakeTimers();
    const setIntervalSpy = vi.spyOn(window, 'setInterval');
    const fixture = create();

    expect(setIntervalSpy).not.toHaveBeenCalled();
    fixture.destroy();
    vi.useRealTimers();
  });

  it('disconnects both shared resources when the window closes', async () => {
    const fixture = create();
    await fixture.whenStable();

    fixture.destroy();

    expect(systemDisconnect).toHaveBeenCalledOnce();
    expect(modelDisconnect).toHaveBeenCalledOnce();
  });
});
