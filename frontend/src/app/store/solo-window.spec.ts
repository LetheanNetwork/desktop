import { DOCUMENT } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { SOLO_APP_WINDOW, isSoloWindowUrl } from './solo-window';

const documentAt = (href: string) => ({ defaultView: { location: { href } } });

describe('isSoloWindowUrl', () => {
  it('recognises the solo application route and nothing else', () => {
    expect(isSoloWindowUrl('wails://localhost/#/w/telemetry')).toBe(true);
    expect(isSoloWindowUrl('wails://localhost/#/w/control/models')).toBe(true);
    expect(isSoloWindowUrl('http://127.0.0.1:9245/#/w/files')).toBe(true);

    expect(isSoloWindowUrl('wails://localhost/#/')).toBe(false);
    expect(isSoloWindowUrl('wails://localhost/#/tray')).toBe(false);
    expect(isSoloWindowUrl('wails://localhost/#/system/control/models')).toBe(false);
    expect(isSoloWindowUrl('wails://localhost/w/telemetry')).toBe(false);
    expect(isSoloWindowUrl('')).toBe(false);
  });
});

describe('SOLO_APP_WINDOW', () => {
  it('reads the document URL, so it is known before the first navigation', () => {
    TestBed.configureTestingModule({
      providers: [{ provide: DOCUMENT, useValue: documentAt('wails://localhost/#/w/telemetry') }],
    });

    expect(TestBed.inject(SOLO_APP_WINDOW)).toBe(true);
  });

  it('is false in the desktop shell window', () => {
    TestBed.configureTestingModule({
      providers: [{ provide: DOCUMENT, useValue: documentAt('wails://localhost/#/') }],
    });

    expect(TestBed.inject(SOLO_APP_WINDOW)).toBe(false);
  });
});
