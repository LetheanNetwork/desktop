import { TestBed } from '@angular/core/testing';
import { Win } from '../../desktop.data';
import {
  CAPABILITY_SESSION_TOKEN,
  ExtensionsMarketplaceSurface,
  discriminateView,
  hasAnyViews,
  sessionTokenPrompt,
  viewSummaryLine,
  wantsSessionTokenCapability,
} from './marketplace';

describe('marketplace view disclosures', () => {
  it('distinguishes integrated, isolated, and account-capable views', () => {
    expect(discriminateView({ Kind: 'lit' })).toMatchObject({
      iconClass: 'fa-cubes',
      label: 'Deeply integrated',
    });
    expect(
      discriminateView({ Kind: 'iframe', Capabilities: [CAPABILITY_SESSION_TOKEN] }),
    ).toMatchObject({
      iconClass: 'fa-shield-halved',
      label: 'Isolated webapp — requests account access',
    });
    expect(discriminateView({ Kind: 'iframe' })).toMatchObject({
      iconClass: 'fa-shield',
      label: 'Isolated webapp',
    });
  });

  it('summarises manifest views and session-token consent accurately', () => {
    const manifest = {
      Plugin: {
        Code: 'opencode',
        Views: [
          {
            ID: 'opencode',
            Label: 'OpenCode',
            Kind: 'iframe',
            Capabilities: [CAPABILITY_SESSION_TOKEN],
          },
          { ID: 'metrics', Label: 'Metrics', Kind: 'lit' },
        ],
      },
    };
    expect(hasAnyViews(manifest)).toBe(true);
    expect(viewSummaryLine(manifest)).toBe('Provides views: OpenCode, Metrics');
    expect(wantsSessionTokenCapability(manifest)).toBe(true);
    expect(sessionTokenPrompt('OpenCode')).toContain(
      'OpenCode can read and write any data you have unlocked',
    );
  });

  it('treats absent and empty manifests as viewless', () => {
    expect(hasAnyViews(undefined)).toBe(false);
    expect(hasAnyViews({ Plugin: { Code: 'empty', Views: [] } })).toBe(false);
    expect(viewSummaryLine(null)).toBe('');
    expect(wantsSessionTokenCapability(null)).toBe(false);
  });
});

describe('ExtensionsMarketplaceSurface', () => {
  const win: Win = {
    id: 'marketplace-window',
    app: 'extensions',
    sub: 'marketplace',
    x: 0,
    y: 0,
    w: 640,
    h: 480,
    z: 1,
    min: false,
    max: false,
  };

  function create() {
    const fixture = TestBed.createComponent(ExtensionsMarketplaceSurface);
    fixture.componentRef.setInput('win', win);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => TestBed.resetTestingModule());

  it('lists the fixed catalogue with install state, discriminators and the account-access prompt', () => {
    const fixture = create();
    const element = fixture.nativeElement as HTMLElement;

    const articles = Array.from(element.querySelectorAll('article'));
    expect(articles).toHaveLength(3);
    expect(articles.map((article) => article.querySelector('h3')?.textContent)).toEqual([
      'OpenCode',
      'Grafana Local',
      'Lethean Lab Tools',
    ]);
    expect(articles[0].querySelector('.state')?.textContent).toBe('installed');
    expect(articles[1].querySelector('.state')?.textContent).toBe('available');

    // OpenCode declares the session-token capability, so its account-access
    // consent prompt must be rendered alongside the isolated-webapp badge.
    expect(articles[0].querySelector('.prompt span')?.textContent).toContain(
      'OpenCode can read and write any data you have unlocked',
    );
    expect(articles[0].querySelector('.discriminator strong')?.textContent).toContain(
      'Isolated webapp — requests account access',
    );
    // Lethean Lab Tools' single view is integrated (kind lit), never isolated.
    expect(articles[2].querySelector('.discriminator strong')?.textContent).toContain(
      'Deeply integrated',
    );
    expect(articles[2].querySelector('.prompt')).toBeNull();
  });

  it('filters the catalogue by search text and shows the empty state for no matches', () => {
    const fixture = create();
    const page = fixture.componentInstance;
    const input = fixture.nativeElement.querySelector('input') as HTMLInputElement;

    input.value = 'grafana';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    expect(page.query()).toBe('grafana');
    expect(
      Array.from(fixture.nativeElement.querySelectorAll('article h3') as NodeListOf<HTMLElement>).map(
        (node) => node.textContent,
      ),
    ).toEqual(['Grafana Local']);

    input.value = 'no such plugin exists';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('article')).toHaveLength(0);
    expect(fixture.nativeElement.querySelector('.empty')?.textContent).toContain(
      'No extensions match this search.',
    );
  });

  it('opens a local review of the selected entry without performing an install', () => {
    const fixture = create();
    const page = fixture.componentInstance;
    const button = fixture.nativeElement.querySelector(
      'article button',
    ) as HTMLButtonElement;

    expect(fixture.nativeElement.querySelector('footer[role="status"]')).toBeNull();

    button.click();
    fixture.detectChanges();

    expect(page.selected()?.code).toBe('opencode');
    const footer = fixture.nativeElement.querySelector('footer[role="status"]') as HTMLElement;
    expect(footer.textContent).toContain('OpenCode');
    expect(footer.textContent).toContain('review opened locally');
  });
});
