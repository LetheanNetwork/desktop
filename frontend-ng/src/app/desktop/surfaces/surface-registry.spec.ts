import { SURFACE_APPS, SURFACE_APP_REGISTRY, SURFACE_CATEGORIES } from './surface-registry';

describe('surface registry', () => {
  it('registers all 43 Angular surface routes exactly once', () => {
    expect(Object.keys(SURFACE_APPS)).toHaveLength(43);
    expect(Object.keys(SURFACE_APP_REGISTRY)).toEqual(Object.keys(SURFACE_APPS));
    expect(SURFACE_CATEGORIES.map(({ id }) => id)).toEqual([
      'agents',
      'coding',
      'marketing',
      'ml-lab',
      'observe',
      'office',
      'operations',
      'planning',
      'sales',
      'extensions',
    ]);
    const routeKeys = Object.values(SURFACE_APPS).map(({ id, route }) => `${id}:${route}`);
    expect(new Set(routeKeys).size).toBe(43);
  });

  it('resolves every lazy loader to a real standalone Angular component', async () => {
    const loaded = await Promise.all(
      Object.entries(SURFACE_APP_REGISTRY).map(
        async ([id, loader]) => [id, await loader()] as const,
      ),
    );
    expect(loaded).toHaveLength(43);
    for (const [id, component] of loaded) {
      expect(component, id).toBeTypeOf('function');
      expect((component as unknown as { ɵcmp?: unknown }).ɵcmp, id).toBeDefined();
    }
  });
});
