import { TestBed } from '@angular/core/testing';
import { TRAY_PANEL_RUNTIME, TrayPanel, TrayPanelRuntime } from './tray-panel';

describe('TrayPanel', () => {
  const defaultCall = async (method: string): Promise<unknown> => {
    if (method.endsWith('.Status')) {
      return {
        model_path: '/Users/test/Models/gemma-4b',
        runtime: 'mlx',
        loaded_at_unix: 1_700_000_000,
        config: { context_length: 8192, parallel_slots: 2 },
      };
    }
    if (method.endsWith('.CurrentSample')) {
      return {
        OK: true,
        Value: {
          heap_alloc_mb: 128.4,
          uptime_seconds: 7_380,
          num_goroutines: 42,
          last_gc_pause_ms: 0.7,
        },
      };
    }
    return { OK: true, Value: null };
  };

  const runtime: TrayPanelRuntime = {
    call: vi.fn(defaultCall),
    emit: vi.fn(async () => false),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(runtime.call).mockImplementation(defaultCall);
    vi.mocked(runtime.emit).mockResolvedValue(false);
    TestBed.configureTestingModule({
      imports: [TrayPanel],
      providers: [{ provide: TRAY_PANEL_RUNTIME, useValue: runtime }],
    });
  });

  it('renders the live model and process summary in one compact surface', async () => {
    const fixture = TestBed.createComponent(TrayPanel);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelector('[data-testid="model-name"]')?.textContent).toContain('gemma-4b');
    expect(element.querySelector('[data-testid="heap"]')?.textContent).toContain('128.4 MB');
    expect(element.querySelector('[data-testid="uptime"]')?.textContent).toContain('2h 3m');
    expect(element.querySelectorAll('[data-testid="quick-action"]')).toHaveLength(3);
  });

  it('opens the desktop target, emits its navigation event, then dismisses the panel', async () => {
    const fixture = TestBed.createComponent(TrayPanel);
    await fixture.whenStable();

    const chat = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>(
      '[data-target="chat"]',
    );
    chat?.click();
    await fixture.whenStable();

    expect(runtime.call).toHaveBeenCalledWith(
      'dappco.re/go/render/display/webkit.WindowBindingService.Open',
      ['app'],
    );
    expect(runtime.emit).toHaveBeenCalledWith('lthn:tray:open', 'chat');
    expect(runtime.call).toHaveBeenCalledWith(
      'dappco.re/go/render/display/webkit.WindowBindingService.Hide',
      ['tray-panel'],
    );
  });

  it('does not navigate or dismiss when the desktop window cannot open', async () => {
    vi.mocked(runtime.call).mockImplementation(async (method: string) => {
      if (method.endsWith('.Open')) {
        return { OK: false, Value: { message: 'desktop window unavailable' } };
      }
      return defaultCall(method);
    });
    const fixture = TestBed.createComponent(TrayPanel);
    await fixture.whenStable();

    const component = fixture.componentInstance;
    await component.open('chat');

    expect(runtime.emit).not.toHaveBeenCalled();
    expect(runtime.call).not.toHaveBeenCalledWith(
      'dappco.re/go/render/display/webkit.WindowBindingService.Hide',
      ['tray-panel'],
    );
    expect(component.statusMessage()).toContain('desktop window unavailable');
  });

  it('keeps the panel usable when the local model service is unavailable', async () => {
    vi.mocked(runtime.call).mockImplementationOnce(async () => {
      throw new Error('offline');
    });
    const fixture = TestBed.createComponent(TrayPanel);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelector('[data-testid="model-empty"]')?.textContent).toContain(
      'No model loaded',
    );
    expect(element.querySelector('[data-target="models"]')).not.toBeNull();
  });

  it('finishes refreshing when process telemetry returns a failed core result', async () => {
    vi.mocked(runtime.call).mockImplementation(async (method: string) => {
      if (method.endsWith('.CurrentSample')) {
        return { OK: false, Value: { message: 'telemetry unavailable' } };
      }
      return defaultCall(method);
    });
    const fixture = TestBed.createComponent(TrayPanel);

    await fixture.whenStable();

    expect(fixture.componentInstance.loading()).toBe(false);
    expect(fixture.componentInstance.telemetry()).toBeNull();
    await fixture.componentInstance.refresh();
    expect(runtime.call).toHaveBeenCalledTimes(4);
  });
});
