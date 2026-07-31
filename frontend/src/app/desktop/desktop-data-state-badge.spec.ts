import { TestBed } from '@angular/core/testing';
import { DesktopDataStateBadge } from './desktop-data-state-badge';

describe('DesktopDataStateBadge', () => {
  it('renders the canonical label, variant, and machine-readable state', async () => {
    const fixture = TestBed.createComponent(DesktopDataStateBadge);
    fixture.componentRef.setInput('state', 'mixed');

    await fixture.whenStable();

    const badge = (fixture.nativeElement as HTMLElement).querySelector('lthn-badge');
    expect(badge?.textContent).toContain('Live + demo');
    expect(badge?.getAttribute('variant')).toBe('warn');
    expect((badge as HTMLElement | null)?.dataset['dataState']).toBe('mixed');
  });

  it('renders a stale live source as a warning', async () => {
    const fixture = TestBed.createComponent(DesktopDataStateBadge);
    fixture.componentRef.setInput('state', 'stale');

    await fixture.whenStable();

    const badge = (fixture.nativeElement as HTMLElement).querySelector('lthn-badge');
    expect(badge?.textContent).toContain('Live data stale');
    expect(badge?.getAttribute('variant')).toBe('warn');
    expect((badge as HTMLElement | null)?.dataset['dataState']).toBe('stale');
  });
});
