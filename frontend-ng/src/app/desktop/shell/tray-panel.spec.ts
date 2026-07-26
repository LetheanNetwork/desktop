import { TestBed } from '@angular/core/testing';
import { ShellTrayPanel } from './tray-panel';

describe('ShellTrayPanel', () => {
  it('renders tray variants and emits language and application intents', async () => {
    const fixture = TestBed.createComponent(ShellTrayPanel);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('trayKey', 'lang');
    fixture.componentRef.setInput('left', 400);
    fixture.componentRef.setInput('top', 28);
    fixture.componentRef.setInput('languages', [
      ['en', 'English'],
      ['fr', 'Français'],
    ]);
    fixture.componentRef.setInput('language', 'en');
    fixture.componentRef.setInput('clockText', '13:38');
    fixture.componentRef.setInput('dateText', 'Saturday, 26 July');
    fixture.componentRef.setInput('worldClocks', [{ city: 'London', time: '13:38' }]);
    fixture.componentRef.setInput('wattsJson', '[190,207]');

    const languages: string[] = [];
    const apps: Array<{ appId: string; subId?: string }> = [];
    fixture.componentInstance.languageRequested.subscribe((code) => languages.push(code));
    fixture.componentInstance.appRequested.subscribe((request) => apps.push(request));

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const languageRows = element.querySelectorAll<HTMLElement>('.traypanel .mi');

    expect(element.querySelector('.trh')?.textContent).toContain('Language');
    expect(languageRows[0]?.textContent).toContain('English');
    expect(languageRows[1]?.textContent).toContain('Français');
    expect(fixture.componentInstance.panelElement).toBe(
      element.querySelector<HTMLElement>('.traypanel'),
    );

    languageRows[1]?.click();
    await fixture.whenStable();

    fixture.componentRef.setInput('trayKey', 'battery');
    await fixture.whenStable();
    const powerButton = element.querySelector<HTMLButtonElement>('.tr-btn');
    expect(element.querySelector('.trh')?.textContent).toContain('Power');
    expect(element.querySelector('lthn-sparkline')?.getAttribute('data')).toBe('[190,207]');
    powerButton?.click();
    await fixture.whenStable();

    expect(languages).toEqual(['fr']);
    expect(apps).toEqual([{ appId: 'control', subId: 'power' }]);
  });
});
