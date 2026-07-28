// SPDX-License-Identifier: EUPL-1.2

import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from './desktop-data-resource';

interface Reading {
  readonly value: number;
}

describe('desktop data resource', () => {
  it('creates ready demo data without a receipt timestamp or retry', () => {
    expect(createDemoResource<Reading>({ value: 41 }, 'Lethean demo fixture')).toEqual({
      mode: 'demo',
      state: 'demo',
      source: 'Lethean demo fixture',
      updatedAt: null,
      refreshing: false,
      error: null,
      canRetry: false,
      value: { value: 41 },
    });
  });

  it('creates an empty connected loading resource', () => {
    expect(createConnectedResource<Reading>('Local process runtime')).toEqual({
      mode: 'connected',
      state: 'loading',
      source: 'Local process runtime',
      updatedAt: null,
      refreshing: false,
      error: null,
      canRetry: false,
      value: null,
    });
  });

  it('begins an initial refresh without mutating the input', () => {
    const resource = createConnectedResource<Reading>('Local process runtime');
    const next = beginDesktopDataRefresh(resource, 1_000, 10_000);

    expect(next).not.toBe(resource);
    expect(resource.refreshing).toBe(false);
    expect(next).toMatchObject({
      state: 'loading',
      refreshing: true,
      error: null,
      canRetry: false,
    });
  });

  it('retains a current value during refresh and marks an overdue value stale', () => {
    const current: DesktopDataResource<Reading> = {
      mode: 'connected',
      state: 'live',
      source: 'Local process runtime',
      updatedAt: 1_000,
      refreshing: false,
      error: 'old reader message',
      canRetry: true,
      value: { value: 41 },
    };

    expect(beginDesktopDataRefresh(current, 10_999, 10_000)).toMatchObject({
      state: 'live',
      value: { value: 41 },
      refreshing: true,
      error: null,
      canRetry: false,
    });
    expect(beginDesktopDataRefresh(current, 11_001, 10_000)).toMatchObject({
      state: 'stale',
      value: { value: 41 },
      refreshing: true,
    });
  });

  it.each(['live', 'mixed'] as const)(
    'resolves an active refresh to %s and records local receipt metadata',
    (state) => {
      const refreshing = beginDesktopDataRefresh(
        createConnectedResource<Reading>('Local process runtime'),
        1_000,
        10_000,
      );
      const next = resolveDesktopData(
        refreshing,
        { value: 42 },
        state,
        'Local process runtime',
        2_000,
      );

      expect(next).toEqual({
        mode: 'connected',
        state,
        source: 'Local process runtime',
        updatedAt: 2_000,
        refreshing: false,
        error: null,
        canRetry: false,
        value: { value: 42 },
      });
    },
  );

  it('turns a first rejection into unavailable data with Retry enabled', () => {
    const refreshing = beginDesktopDataRefresh(
      createConnectedResource<Reading>('Local process runtime'),
      1_000,
      10_000,
    );

    expect(rejectDesktopData(refreshing, 'Live data is unavailable.')).toEqual({
      mode: 'connected',
      state: 'unavailable',
      source: 'Local process runtime',
      updatedAt: null,
      refreshing: false,
      error: 'Live data is unavailable.',
      canRetry: true,
      value: null,
    });
  });

  it('turns a later rejection stale without losing value, source, or receipt time', () => {
    const refreshing: DesktopDataResource<Reading> = {
      mode: 'connected',
      state: 'mixed',
      source: 'Local process runtime',
      updatedAt: 2_000,
      refreshing: true,
      error: null,
      canRetry: false,
      value: { value: 42 },
    };
    const next = rejectDesktopData(refreshing, 'Refresh failed.');

    expect(next).toEqual({
      ...refreshing,
      state: 'stale',
      refreshing: false,
      error: 'Refresh failed.',
      canRetry: true,
    });
    expect(next.value).toBe(refreshing.value);
  });

  it('recovers stale data and clears its error and Retry state', () => {
    const stale: DesktopDataResource<Reading> = {
      mode: 'connected',
      state: 'stale',
      source: 'Local process runtime',
      updatedAt: 2_000,
      refreshing: false,
      error: 'Refresh failed.',
      canRetry: true,
      value: { value: 42 },
    };
    const refreshing = beginDesktopDataRefresh(stale, 3_000, 10_000);

    expect(
      resolveDesktopData(refreshing, { value: 43 }, 'live', 'Local process runtime', 4_000),
    ).toMatchObject({
      state: 'live',
      updatedAt: 4_000,
      refreshing: false,
      error: null,
      canRetry: false,
      value: { value: 43 },
    });
  });

  it('rejects demo refreshes, overlapping refreshes, and inactive settlements', () => {
    const demo = createDemoResource<Reading>({ value: 41 }, 'Demo');
    const refreshingDemo = { ...demo, refreshing: true };
    const connected = createConnectedResource<Reading>('Live');
    const refreshing = beginDesktopDataRefresh(connected, 1_000, 10_000);

    expect(() => beginDesktopDataRefresh(demo, 1_000, 10_000)).toThrow('connected resource');
    expect(() => resolveDesktopData(refreshingDemo, { value: 42 }, 'live', 'Live', 2_000)).toThrow(
      'connected resource',
    );
    expect(() => rejectDesktopData(refreshingDemo, 'failed')).toThrow('connected resource');
    expect(() => beginDesktopDataRefresh(refreshing, 1_000, 10_000)).toThrow('already refreshing');
    expect(() => resolveDesktopData(connected, { value: 1 }, 'live', 'Live', 1_000)).toThrow(
      'active refresh',
    );
    expect(() => rejectDesktopData(connected, 'failed')).toThrow('active refresh');
  });

  it('rejects impossible result states and null successful values', () => {
    const refreshing = beginDesktopDataRefresh(
      createConnectedResource<Reading>('Live'),
      1_000,
      10_000,
    );

    expect(() =>
      resolveDesktopData(refreshing, { value: 1 }, 'stale' as 'live', 'Live', 2_000),
    ).toThrow('live or mixed');
    expect(() => resolveDesktopData(refreshing, null as never, 'live', 'Live', 2_000)).toThrow(
      'non-null value',
    );
    expect(() => createDemoResource<Reading>(null as never, 'Demo')).toThrow('non-null value');
  });

  it('rejects manually constructed live-null and unavailable-valued resources', () => {
    const connected = createConnectedResource<Reading>('Live');
    const liveWithoutValue = {
      ...connected,
      state: 'live' as const,
    };
    const unavailableWithValue = {
      ...connected,
      state: 'unavailable' as const,
      value: { value: 1 },
    };

    expect(() => beginDesktopDataRefresh(liveWithoutValue, 1_000, 10_000)).toThrow(
      'non-null value',
    );
    expect(() => beginDesktopDataRefresh(unavailableWithValue, 1_000, 10_000)).toThrow(
      'null value',
    );
  });
});
