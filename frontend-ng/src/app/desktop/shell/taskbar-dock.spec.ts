import { TestBed } from '@angular/core/testing';
import type { AppDef, Win } from '../desktop.data';
import type { ShellWindowGroup } from './shell.types';
import { ShellTaskbarDock } from './taskbar-dock';

const apps: Record<string, AppDef> = {
  control: {
    id: 'control',
    title: 'Control',
    icon: 'cube',
    w: 780,
    h: 560,
    hint: 'Local models',
  },
  telemetry: {
    id: 'telemetry',
    title: 'Telemetry',
    icon: 'wave-square',
    w: 660,
    h: 400,
    hint: 'Runtime telemetry',
  },
};

const taskWins: Win[] = [
  {
    id: 'control-window',
    app: 'control',
    sub: '',
    x: 70,
    y: 24,
    w: 780,
    h: 560,
    z: 11,
    min: false,
    max: false,
  },
  {
    id: 'telemetry-window',
    app: 'telemetry',
    sub: '',
    x: 104,
    y: 54,
    w: 660,
    h: 400,
    z: 12,
    min: true,
    max: false,
  },
];

describe('ShellTaskbarDock', () => {
  it('renders session, task, process, group, and Trash controls with their original intents', async () => {
    const group: ShellWindowGroup = {
      id: 'observe',
      name: 'Observe',
      ids: ['control-window', 'telemetry-window'],
      apps: ['control', 'telemetry'],
      open: true,
    };
    const fixture = TestBed.createComponent(ShellTaskbarDock);
    fixture.componentRef.setInput('user', {
      initials: 'SR',
      name: 'Sarah Reeve',
      email: 'sarah@lethean.local',
      host: 'lethean.local',
    });
    fixture.componentRef.setInput('taskWins', taskWins);
    fixture.componentRef.setInput('focusId', 'control-window');
    fixture.componentRef.setInput('apps', apps);
    fixture.componentRef.setInput('runningApps', ['control']);
    fixture.componentRef.setInput('groups', [group]);
    fixture.componentRef.setInput('dropzone', true);

    const sessions: Event[] = [];
    const tasks: string[] = [];
    const docks: string[] = [];
    const dockContexts: string[] = [];
    const groups: string[] = [];
    const groupContexts: ShellWindowGroup[] = [];
    fixture.componentInstance.sessionRequested.subscribe((event) => sessions.push(event));
    fixture.componentInstance.taskRequested.subscribe((id) => tasks.push(id));
    fixture.componentInstance.dockRequested.subscribe((id) => docks.push(id));
    fixture.componentInstance.dockContextRequested.subscribe(({ value }) =>
      dockContexts.push(value),
    );
    fixture.componentInstance.groupRequested.subscribe((id) => groups.push(id));
    fixture.componentInstance.groupContextRequested.subscribe(({ value }) =>
      groupContexts.push(value),
    );

    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const taskButtons = element.querySelectorAll<HTMLButtonElement>('.task');
    const processButton = element.querySelector<HTMLButtonElement>(
      '.dock .di:not(.group):not([aria-label="Trash"])',
    );
    const groupButton = element.querySelector<HTMLButtonElement>('.dock .di.group');

    expect(element.querySelector('.session .who')?.textContent).toContain('Sarah Reeve');
    expect(Array.from(taskButtons, (button) => button.textContent?.trim())).toEqual([
      'Control',
      'Telemetry',
    ]);
    expect(taskButtons[0].classList.contains('active')).toBe(true);
    expect(element.querySelector('.dock')?.classList.contains('dropzone')).toBe(true);
    expect(element.querySelector('[aria-label="Trash"]')).not.toBeNull();

    element.querySelector<HTMLButtonElement>('.session')?.click();
    taskButtons[0].click();
    processButton?.click();
    processButton?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true }));
    groupButton?.click();
    groupButton?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true }));
    await fixture.whenStable();

    expect(sessions).toHaveLength(1);
    expect(tasks).toEqual(['control-window']);
    expect(docks).toEqual(['control']);
    expect(dockContexts).toEqual(['control']);
    expect(groups).toEqual(['observe']);
    expect(groupContexts).toEqual([group]);
  });
});
