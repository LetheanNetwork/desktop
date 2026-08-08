import { APP_NAV, APPS, CATEGORIES, ORDER } from './desktop-catalogue.data';
import { APP_PANES, panesFor } from './desktop-panes.data';
import { devPanelFor } from './dev-panel.data';

describe('desktop application catalogue', () => {
  it('publishes twenty-four applications across the seven launcher categories', () => {
    expect(Object.keys(APPS)).toEqual([
      'welcome',
      'control',
      'telemetry',
      'activity',
      'operations',
      'marketplace',
      'settings',
      'ide',
      'scm',
      'databases',
      'project-manager',
      'mail',
      'documents',
      'crm',
      'marketing',
      'tenant',
      'chat',
      'agents',
      'ml-lab',
      'opencode',
      'games',
      'files',
      'notepad',
      'lethernet',
    ]);
    expect(CATEGORIES.map(({ id, apps }) => [id, apps])).toEqual([
      [
        'system',
        ['welcome', 'control', 'telemetry', 'activity', 'operations', 'marketplace', 'settings'],
      ],
      ['developer', ['ide', 'scm', 'databases']],
      ['office', ['project-manager', 'mail', 'documents', 'crm', 'marketing', 'tenant']],
      ['ai', ['chat', 'agents', 'ml-lab', 'opencode']],
      ['media', ['games']],
      ['tools', ['files', 'notepad']],
      ['networking', ['lethernet']],
    ]);
  });

  it('resolves every ordered and categorised application reference', () => {
    const danglingOrderIds = ORDER.filter((id) => !APPS[id]);
    const danglingCategoryIds = CATEGORIES.flatMap((category) =>
      category.apps.filter((id) => !APPS[id]).map((id) => `${category.id}:${id}`),
    );

    expect(danglingOrderIds).toEqual([]);
    expect(danglingCategoryIds).toEqual([]);
    // Welcome opens itself on a fresh desktop; it is not a launcher icon.
    expect(ORDER).toEqual(Object.keys(APPS).filter((id) => id !== 'welcome'));
  });

  it('keeps the desktop icons to the nine everyday applications', () => {
    expect(ORDER.filter((id) => !APPS[id].dev)).toEqual([
      'control',
      'telemetry',
      'activity',
      'settings',
      'chat',
      'games',
      'files',
      'notepad',
      'lethernet',
    ]);
  });

  it('opens every panelled application on a pane it actually publishes', () => {
    const mismatched = Object.values(APPS)
      .filter((app) => (APP_NAV[app.id] ?? []).length > 0)
      .filter((app) => !(APP_NAV[app.id] ?? []).some(([path]) => path === app.defaultSub))
      .map(({ id }) => id);

    expect(mismatched).toEqual([]);
    // A pane-backed application opens on its first pane.
    const outOfOrder = [...new Set(APP_PANES.map(({ app }) => app))].filter(
      (app) => APPS[app].defaultSub !== panesFor(app)[0].path,
    );
    expect(outOfOrder).toEqual([]);
  });

  it('provides a fixture for every developer panel route', () => {
    const missingPaneFixtures = APP_PANES.filter(({ dev }) => dev)
      .filter(({ dev }) => devPanelFor(dev!).kind === 'empty')
      .map(({ app, path }) => `${app}/${path}`);
    const missingApplicationFixtures = Object.values(APPS)
      .filter((app) => app.route)
      .filter((app) => devPanelFor(app.route!).kind === 'empty')
      .map((app) => app.id);

    expect(missingPaneFixtures).toEqual([]);
    expect(missingApplicationFixtures).toEqual([]);
  });
});
