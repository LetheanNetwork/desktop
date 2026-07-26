import { TestBed } from '@angular/core/testing';
import type { DesktopMenuApp, DesktopMenuCategory } from '../desktop-route-tree';
import { ShellStartMenu } from './start-menu';

const app = {
  id: 'settings',
  path: 'settings',
  category: 'system',
  categoryPath: 'system',
  title: 'Settings',
  icon: 'sliders',
  hint: 'System preferences',
  children: [['translations', 'language', 'Translations']],
  loadComponent: async () => {
    throw new Error('The presentation test must not load routed applications');
  },
} as DesktopMenuApp;

const category: DesktopMenuCategory = {
  id: 'system',
  path: 'system',
  title: 'System',
  icon: 'gear',
  apps: [app],
};

describe('ShellStartMenu', () => {
  it('renders the existing launcher and emits category, app, child, and session intents', async () => {
    const fixture = TestBed.createComponent(ShellStartMenu);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('position', { left: 18, top: null, bottom: 56 });
    fixture.componentRef.setInput('categories', [category]);
    fixture.componentRef.setInput('openCategories', { system: true });
    fixture.componentRef.setInput('user', {
      initials: 'SR',
      name: 'Sarah Reeve',
      email: 'sarah@lethean.local',
      host: 'lethean.local',
    });
    fixture.componentRef.setInput('submenuOpen', true);
    fixture.componentRef.setInput('submenuLeft', 224);
    fixture.componentRef.setInput('submenuTop', 72);
    fixture.componentRef.setInput('submenuParent', 'settings');
    fixture.componentRef.setInput('submenuItems', app.children);

    const categoryRequests: string[] = [];
    const appRequests: DesktopMenuApp[] = [];
    const appHovers: DesktopMenuApp[] = [];
    const childRequests: Array<{ appId: string; childId: string }> = [];
    const sessionRequests: string[] = [];
    fixture.componentInstance.categoryRequested.subscribe((id) => categoryRequests.push(id));
    fixture.componentInstance.appRequested.subscribe((value) => appRequests.push(value));
    fixture.componentInstance.appHovered.subscribe(({ value }) => appHovers.push(value));
    fixture.componentInstance.childRequested.subscribe((value) => childRequests.push(value));
    fixture.componentInstance.sessionRequested.subscribe(({ value }) =>
      sessionRequests.push(value),
    );

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const categoryRow = element.querySelector<HTMLElement>('.sm-cat');
    const appRow = element.querySelector<HTMLElement>('.sm-child');
    const childRow = element.querySelector<HTMLElement>('.submenu .mi');
    const sessionRows = element.querySelectorAll<HTMLElement>('.sp-user > .mi');

    expect(element.querySelector('.sp-h')?.textContent?.trim()).toBe('Programs');
    expect(categoryRow?.textContent).toContain('System');
    expect(appRow?.textContent).toContain('Settings');
    expect(element.querySelector('.mhead')?.textContent).toContain('Sarah Reeve');
    expect(childRow?.textContent).toContain('Translations');
    expect(fixture.componentInstance.panelElement).toBe(
      element.querySelector<HTMLElement>('.sessionmenu'),
    );

    categoryRow?.click();
    appRow?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    appRow?.click();
    childRow?.click();
    sessionRows[0]?.click();
    await fixture.whenStable();

    expect(categoryRequests).toEqual(['system']);
    expect(appRequests).toEqual([app]);
    expect(appHovers).toEqual([app]);
    expect(childRequests).toEqual([{ appId: 'settings', childId: 'translations' }]);
    expect(sessionRequests).toEqual(['lock']);
  });
});
