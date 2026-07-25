import { ComponentFixture } from '@angular/core/testing';
import { TestBed } from '@angular/core/testing';
import { HashLocationStrategy, LocationStrategy } from '@angular/common';
import { Router } from '@angular/router';
import { App } from './app';
import { appConfig } from './app.config';
import { StorageService } from './store/storage.service';

describe('App first paint', () => {
  let fixture: ComponentFixture<App>;
  let router: Router;
  const storage = {
    read: vi.fn(() => null),
    write: vi.fn(),
  };

  beforeEach(async () => {
    storage.read.mockClear();
    storage.write.mockClear();

    await TestBed.configureTestingModule({
      imports: [App],
      providers: [...appConfig.providers, { provide: StorageService, useValue: storage }],
    }).compileComponents();

    fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    router = TestBed.inject(Router);
    await router.navigateByUrl('/');
  });

  afterEach(() => {
    fixture.destroy();
  });

  it('renders hash-routed desktop chrome and startup-seeded windows', async () => {
    const element = fixture.nativeElement as HTMLElement;

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(router.url).toBe('/system/telemetry');
      expect(TestBed.inject(LocationStrategy)).toBeInstanceOf(HashLocationStrategy);
      expect(TestBed.inject(LocationStrategy).prepareExternalUrl('/')).toBe('#/');
      expect(TestBed.inject(LocationStrategy).prepareExternalUrl(router.url)).toBe(
        '#/system/telemetry',
      );
      expect(element.querySelector('lthn-desktop #os')).not.toBeNull();
      expect(element.querySelector('.menubar')).not.toBeNull();
      expect(element.querySelector('.bar')).not.toBeNull();
      expect(element.querySelector('.dock')).not.toBeNull();
      expect(element.querySelectorAll('#winlayer > .win')).toHaveLength(2);
      expect(element.querySelector('lthn-control-app')).not.toBeNull();
      expect(element.querySelector('lthn-telemetry-app')).not.toBeNull();
      expect(element.querySelector('app-app-shell')).toBeNull();
      expect(element.querySelector('router-outlet')).not.toBeNull();
    });

    const desktop = element.querySelector('lthn-desktop');
    expect(desktop).not.toBeNull();
    expect(getComputedStyle(desktop!).display).toBe('flex');
    expect(getComputedStyle(desktop!).height).toBe('100%');
    expect(storage.read).toHaveBeenCalledWith('lthn.desktop');
  });
});
