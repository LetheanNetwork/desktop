import { APPS, CATEGORIES, ORDER } from './desktop-catalogue.data';
import { devPanelFor } from './dev-panel.data';

describe('desktop application catalogue', () => {
  it('resolves every ordered and categorised application reference', () => {
    const danglingOrderIds = ORDER.filter((id) => !APPS[id]);
    const danglingCategoryIds = CATEGORIES.flatMap((category) =>
      category.apps.filter((id) => !APPS[id]).map((id) => `${category.id}:${id}`),
    );

    expect(danglingOrderIds).toEqual([]);
    expect(danglingCategoryIds).toEqual([]);
  });

  it('provides a fixture for every developer application route', () => {
    const missingFixtures = Object.values(APPS)
      .filter((app) => app.dev)
      .filter((app) => !app.route || devPanelFor(app.route).kind === 'empty')
      .map((app) => app.id);

    expect(missingFixtures).toEqual([]);
  });
});
