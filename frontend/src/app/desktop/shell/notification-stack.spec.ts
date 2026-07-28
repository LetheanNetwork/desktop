import { TestBed } from '@angular/core/testing';
import { ShellNotificationStack } from './notification-stack';

describe('ShellNotificationStack', () => {
  it('renders notification content and emits the dismissed notification id', async () => {
    const fixture = TestBed.createComponent(ShellNotificationStack);
    fixture.componentRef.setInput('notifications', [
      { id: 7, icon: 'circle-info', title: 'Connected', body: 'LetherNet is online' },
      { id: 8, icon: 'check', title: 'Saved', body: '' },
    ]);
    const dismissed: number[] = [];
    fixture.componentInstance.dismissRequested.subscribe((id) => dismissed.push(id));

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const cards = element.querySelectorAll<HTMLElement>('.notif');
    const buttons = element.querySelectorAll<HTMLButtonElement>('.nx');

    expect(cards).toHaveLength(2);
    expect(cards[0].textContent).toContain('Connected');
    expect(cards[0].textContent).toContain('LetherNet is online');
    expect(cards[1].querySelector('small')).toBeNull();

    buttons[0]?.click();
    await fixture.whenStable();

    expect(dismissed).toEqual([7]);
  });
});
