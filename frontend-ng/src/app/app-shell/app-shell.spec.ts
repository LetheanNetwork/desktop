import { ComponentFixture, TestBed } from '@angular/core/testing';

import { AppShell } from './app-shell';

describe('AppShell', () => {
  let component: AppShell;
  let fixture: ComponentFixture<AppShell>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AppShell],
    }).compileComponents();

    fixture = TestBed.createComponent(AppShell);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('renders the lightweight OS chrome', () => {
    expect(component).toBeTruthy();
    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelector('.menubar')?.textContent).toContain('Lethean');
    expect(element.querySelector('.window')).not.toBeNull();
    expect(element.querySelectorAll('.dock i')).toHaveLength(5);
    expect(element.querySelector('[role="status"]')?.getAttribute('aria-label')).toBe(
      'Loading Lethean Desktop',
    );
  });
});
