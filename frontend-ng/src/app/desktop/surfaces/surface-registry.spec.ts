import { signal, type Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../../connection-manager.service';
import { DesktopFilesBridgeService } from '../desktop-files-bridge.service';
import { DEFAULT_DESKTOP_DATA, type Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { SURFACE_APPS, SURFACE_APP_REGISTRY, SURFACE_CATEGORIES } from './surface-registry';

describe('surface registry', () => {
  it('registers all 43 Angular surface routes exactly once', () => {
    expect(Object.keys(SURFACE_APPS)).toHaveLength(43);
    expect(Object.keys(SURFACE_APP_REGISTRY)).toEqual(Object.keys(SURFACE_APPS));
    expect(SURFACE_CATEGORIES.map(({ id }) => id)).toEqual([
      'agents',
      'coding',
      'marketing',
      'ml-lab',
      'observe',
      'office',
      'operations',
      'planning',
      'sales',
      'extensions',
    ]);
    const routeKeys = Object.values(SURFACE_APPS).map(({ id, route }) => `${id}:${route}`);
    expect(new Set(routeKeys).size).toBe(43);
  });

  it('resolves every lazy loader to a real standalone Angular component', async () => {
    const loaded = await Promise.all(
      Object.entries(SURFACE_APP_REGISTRY).map(
        async ([id, loader]) => [id, await loader()] as const,
      ),
    );
    expect(loaded).toHaveLength(43);
    for (const [id, component] of loaded) {
      expect(component, id).toBeTypeOf('function');
      expect((component as unknown as { ɵcmp?: unknown }).ɵcmp, id).toBeDefined();
    }
  });

  it('reuses canonical Files for the Office catalogue route without fixture metrics', async () => {
    const offline = signal(true);
    const registerTool = vi.fn();
    Object.defineProperty(document, 'modelContext', {
      configurable: true,
      value: { registerTool },
    });
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        {
          provide: DesktopFilesBridgeService,
          useValue: {
            listMounts: vi.fn(),
            listDirectory: vi.fn(),
            listTrash: vi.fn(),
            preview: vi.fn(),
            onChanged: vi.fn(),
          },
        },
        {
          provide: WindowManagerService,
          useValue: {
            setSub: vi.fn(),
            setSysTab: vi.fn(),
          },
        },
      ],
    });
    const component = (await SURFACE_APP_REGISTRY['surface-office-files']()) as Type<unknown>;
    const fixture = TestBed.createComponent(component);
    const win: Win = {
      id: 'office-files-window',
      app: 'surface-office-files',
      sub: 'home',
      systab: 'grid',
      x: 0,
      y: 0,
      w: 760,
      h: 520,
      z: 1,
      min: false,
      max: false,
    };
    fixture.componentRef.setInput('win', win);
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;

    expect(element.querySelector('lthn-files-app')).not.toBeNull();
    expect(element.textContent).toContain('Demo data');
    expect(element.textContent).toContain('Documents');
    expect(element.textContent).toContain('welcome.txt');
    expect(element.querySelector('lthn-surface-page')).toBeNull();
    expect(element.textContent).not.toContain('~/Documents · 5 recent');
    expect(registerTool).not.toHaveBeenCalled();
  });

  it('removes the old filesystem fixture from shared desktop data', () => {
    expect('fs' in DEFAULT_DESKTOP_DATA).toBe(false);
  });
});
