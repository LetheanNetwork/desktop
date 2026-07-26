import { TestBed } from '@angular/core/testing';
import { ShellContextMenu } from './context-menu';

describe('ShellContextMenu', () => {
  it('renders nested items and emits submenu, dismissal, and action intents', async () => {
    const action = { label: 'Open', icon: 'arrow-up-right-from-square' };
    const child = { label: 'Dark', icon: 'moon' };
    const parent = { label: 'Appearance', icon: 'palette', children: [child] };
    const fixture = TestBed.createComponent(ShellContextMenu);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('left', 24);
    fixture.componentRef.setInput('top', 32);
    fixture.componentRef.setInput('heading', 'Control');
    fixture.componentRef.setInput('items', [action, { sep: true }, parent]);
    fixture.componentRef.setInput('submenuOpen', true);
    fixture.componentRef.setInput('submenuLeft', 180);
    fixture.componentRef.setInput('submenuTop', 16);
    fixture.componentRef.setInput('submenuIndex', 2);

    const submenuRequests: number[] = [];
    const selected: object[] = [];
    let dismissals = 0;
    fixture.componentInstance.submenuRequested.subscribe(({ value }) =>
      submenuRequests.push(value),
    );
    fixture.componentInstance.submenuDismissed.subscribe(() => dismissals++);
    fixture.componentInstance.itemRequested.subscribe(({ value }) => selected.push(value));

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const actionRow = element.querySelector<HTMLElement>('.ctxmenu > .mi:not(.haschild)');
    const parentRow = element.querySelector<HTMLElement>('.ctxmenu > .mi.haschild');
    const childRow = element.querySelector<HTMLElement>('.ctxsub .mi');

    expect(element.querySelector('.mhead')?.textContent?.trim()).toBe('Control');
    expect(actionRow?.textContent).toContain('Open');
    expect(parentRow?.textContent).toContain('Appearance');
    expect(childRow?.textContent).toContain('Dark');
    expect(fixture.componentInstance.panelElement).toBe(
      element.querySelector<HTMLElement>('.ctxmenu'),
    );

    parentRow?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    actionRow?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    actionRow?.click();
    childRow?.click();
    await fixture.whenStable();

    expect(submenuRequests).toEqual([2]);
    expect(dismissals).toBe(1);
    expect(selected).toEqual([action, child]);
  });
});
