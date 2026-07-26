import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import type { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { FilesApp } from './files.app';

const filesWin: Win = {
  id: 'files-window',
  app: 'files',
  sub: 'home',
  systab: 'list',
  x: 0,
  y: 0,
  w: 760,
  h: 520,
  z: 1,
  min: false,
  max: false,
};

describe('FilesApp', () => {
  const mode = signal<'demo' | 'live'>('demo');
  const liveData = {
    mode: mode.asReadonly(),
    files: vi.fn(),
  };
  const windowManager = {
    setSub: vi.fn(),
    setSysTab: vi.fn(),
  };

  beforeEach(() => {
    mode.set('demo');
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        { provide: DesktopLiveDataService, useValue: liveData },
        { provide: WindowManagerService, useValue: windowManager },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create(win: Win = filesWin) {
    const fixture = TestBed.createComponent(FilesApp);
    fixture.componentRef.setInput('win', { ...win });
    fixture.detectChanges();
    await fixture.whenStable();
    return fixture;
  }

  it('keeps the complete fixture browser available without live calls in demo mode', async () => {
    const fixture = await create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain('Documents');
    expect(text).toContain('welcome.txt');
    expect(text).toContain('218 GB free of 512 GB');
    expect(liveData.files).not.toHaveBeenCalled();
  });

  it('renders saved locations, recent files, and disk usage from the live Files bridge', async () => {
    mode.set('live');
    liveData.files.mockResolvedValue({
      locations: [
        { name: 'Code', count: 12, size: '4.2 GB', brand: false },
        { name: 'Documents', count: 7, size: '180 MB', brand: false },
        { name: 'lthn / models', count: 3, size: '9.8 GB', brand: true },
      ],
      recent: [
        {
          name: 'desktop.data.ts',
          path: '~/Code/lthn/desktop/',
          when: '08:31',
          size: '7 KB',
        },
      ],
      totalRecent: 1,
      disk: { free: '312 GB', total: '1 TB', usedPercent: 68 },
    });

    const fixture = await create();
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live data');
    });
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Code');
    expect(text).toContain('12 items');
    expect(text).toContain('desktop.data.ts');
    expect(text).toContain('312 GB free of 1 TB');
  });

  it('falls back to clearly labelled demo content when the live Files read fails', async () => {
    mode.set('live');
    liveData.files.mockRejectedValue(new Error('files unavailable'));

    const fixture = await create();
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(fixture.componentInstance.dataState()).toBe('unavailable');
    });
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Live unavailable · demo shown');
    expect(text).toContain('Documents');
    expect(text).toContain('welcome.txt');
    expect(text).toContain('218 GB free of 512 GB');
  });
});
