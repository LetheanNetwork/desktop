import { TestBed } from '@angular/core/testing';
import { ShellCommandPalette } from './command-palette';

describe('ShellCommandPalette', () => {
  it('renders results and emits query, keyboard, selection, command, and backdrop intents', async () => {
    const fixture = TestBed.createComponent(ShellCommandPalette);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('query', 'open');
    fixture.componentRef.setInput('selectedIndex', 1);
    fixture.componentRef.setInput('commands', [
      { icon: 'cube', label: 'Open Control', section: 'App', run: () => undefined },
      { icon: 'wave-square', label: 'Open Telemetry', section: 'App', run: () => undefined },
    ]);
    fixture.componentRef.setInput('clockText', '13:38');
    fixture.componentRef.setInput('dateText', 'Saturday, 26 July');
    fixture.componentRef.setInput('throughputJson', '[26,41.8]');
    fixture.componentRef.setInput('wattsJson', '[190,207]');

    const queries: Event[] = [];
    const keys: KeyboardEvent[] = [];
    const selections: number[] = [];
    const commands: number[] = [];
    const backdrops: Event[] = [];
    fixture.componentInstance.queryChanged.subscribe((event) => queries.push(event));
    fixture.componentInstance.keyRequested.subscribe((event) => keys.push(event));
    fixture.componentInstance.selectionRequested.subscribe((index) => selections.push(index));
    fixture.componentInstance.commandRequested.subscribe((index) => commands.push(index));
    fixture.componentInstance.backdropRequested.subscribe((event) => backdrops.push(event));

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const input = element.querySelector<HTMLInputElement>('.pl-in input');
    const rows = element.querySelectorAll<HTMLElement>('.pl-cmd');
    const backdrop = element.querySelector<HTMLElement>('.paletteov');

    expect(input?.value).toBe('open');
    expect(rows[1].classList.contains('on')).toBe(true);
    expect(rows[1].textContent).toContain('Open Telemetry');

    input?.dispatchEvent(new Event('input', { bubbles: true }));
    input?.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    rows[0].dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    rows[0].click();
    backdrop?.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    await fixture.whenStable();

    expect(queries).toHaveLength(1);
    expect(keys.map((event) => event.key)).toEqual(['ArrowDown']);
    expect(selections).toEqual([0]);
    expect(commands).toEqual([0]);
    expect(backdrops).toHaveLength(1);
  });

  it('renders the existing empty-result copy', async () => {
    const fixture = TestBed.createComponent(ShellCommandPalette);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('query', 'missing');
    fixture.componentRef.setInput('selectedIndex', 0);
    fixture.componentRef.setInput('commands', []);
    fixture.componentRef.setInput('clockText', '13:38');
    fixture.componentRef.setInput('dateText', 'Saturday, 26 July');
    fixture.componentRef.setInput('throughputJson', '[26,41.8]');
    fixture.componentRef.setInput('wattsJson', '[190,207]');

    await fixture.whenStable();

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('.pl-empty')?.textContent,
    ).toContain('No commands match “missing”');
  });

  it('retains the time, throughput, and power widgets for an empty query', async () => {
    const fixture = TestBed.createComponent(ShellCommandPalette);
    fixture.componentRef.setInput('open', true);
    fixture.componentRef.setInput('query', '');
    fixture.componentRef.setInput('selectedIndex', 0);
    fixture.componentRef.setInput('commands', []);
    fixture.componentRef.setInput('clockText', '13:38');
    fixture.componentRef.setInput('dateText', 'Saturday, 26 July');
    fixture.componentRef.setInput('throughputJson', '[26,41.8]');
    fixture.componentRef.setInput('wattsJson', '[190,207]');

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const widgets = element.querySelectorAll<HTMLElement>('.plw');
    const sparklines = element.querySelectorAll<HTMLElement>('lthn-sparkline');

    expect(widgets).toHaveLength(3);
    expect(widgets[0].textContent).toContain('13:38');
    expect(widgets[0].textContent).toContain('Saturday, 26 July');
    expect(sparklines[0].getAttribute('data')).toBe('[26,41.8]');
    expect(sparklines[1].getAttribute('data')).toBe('[190,207]');
  });
});
