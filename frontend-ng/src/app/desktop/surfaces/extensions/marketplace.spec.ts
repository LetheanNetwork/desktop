import {
  CAPABILITY_SESSION_TOKEN,
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
