import { TestBed } from '@angular/core/testing';
import type { DesktopMenuCategory } from '../desktop-route-tree';
import { ShellMenuBar } from './menu-bar';

const category: DesktopMenuCategory = {
  id: 'system',
  path: 'system',
  title: 'System',
  icon: 'gear',
  apps: [],
};

describe('ShellMenuBar', () => {
  it('renders the existing chrome and emits the selected menu, category, and tray intents', async () => {
    const fixture = TestBed.createComponent(ShellMenuBar);
    fixture.componentRef.setInput('activeMenuKey', 'system');
    fixture.componentRef.setInput('activeAppTitle', 'Control');
    fixture.componentRef.setInput('categories', [category]);
    fixture.componentRef.setInput('language', 'en');
    fixture.componentRef.setInput('clockText', '13:38');

    const menus: string[] = [];
    const hovers: string[] = [];
    const categories: DesktopMenuCategory[] = [];
    const trays: string[] = [];
    fixture.componentInstance.menuRequested.subscribe(({ value }) => menus.push(value));
    fixture.componentInstance.menuHovered.subscribe(({ value }) => hovers.push(value));
    fixture.componentInstance.categoryRequested.subscribe(({ value }) => categories.push(value));
    fixture.componentInstance.trayRequested.subscribe(({ value }) => trays.push(value));

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const systemMenu = element.querySelector<HTMLButtonElement>('.hoplite-btn');
    const appMenu = element.querySelector<HTMLButtonElement>('.mbtn.app');
    const categoryMenu = element.querySelector<HTMLButtonElement>('.route-category');
    const languageTray = element.querySelector<HTMLButtonElement>('[aria-label="Language"]');

    expect(appMenu?.textContent?.trim()).toBe('Control');
    expect(categoryMenu?.textContent?.trim()).toBe('System');
    expect(languageTray?.textContent?.trim()).toBe('EN');
    expect(element.querySelector('.clock')?.textContent?.trim()).toBe('13:38');
    expect(systemMenu?.getAttribute('aria-expanded')).toBe('true');

    systemMenu?.click();
    appMenu?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    categoryMenu?.click();
    languageTray?.click();
    await fixture.whenStable();

    expect(menus).toEqual(['system']);
    expect(hovers).toEqual(['app']);
    expect(categories).toEqual([category]);
    expect(trays).toEqual(['lang']);
  });
});
