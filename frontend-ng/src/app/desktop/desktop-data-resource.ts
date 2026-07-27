// SPDX-License-Identifier: EUPL-1.2

export type DesktopDataResourceMode = 'demo' | 'connected';

export type DesktopDataResourceState =
  'demo' | 'loading' | 'live' | 'mixed' | 'stale' | 'unavailable';

export interface DesktopDataStatus {
  readonly mode: DesktopDataResourceMode;
  readonly state: DesktopDataResourceState;
  readonly source: string;
  readonly updatedAt: number | null;
  readonly refreshing: boolean;
  readonly error: string | null;
  readonly canRetry: boolean;
}

export interface DesktopDataResource<T> extends DesktopDataStatus {
  readonly value: T | null;
}

export function createDemoResource<T>(value: T, source: string): DesktopDataResource<T> {
  requireValue(value);
  return {
    mode: 'demo',
    state: 'demo',
    source,
    updatedAt: null,
    refreshing: false,
    error: null,
    canRetry: false,
    value,
  };
}

export function createConnectedResource<T>(source: string): DesktopDataResource<T> {
  return {
    mode: 'connected',
    state: 'loading',
    source,
    updatedAt: null,
    refreshing: false,
    error: null,
    canRetry: false,
    value: null,
  };
}

export function beginDesktopDataRefresh<T>(
  resource: DesktopDataResource<T>,
  now: number,
  staleAfterMs: number,
): DesktopDataResource<T> {
  requireValidResource(resource);
  requireConnected(resource);
  if (resource.refreshing) {
    throw new Error('The desktop data resource is already refreshing.');
  }
  if (!Number.isFinite(staleAfterMs) || staleAfterMs < 0) {
    throw new Error('The stale threshold must be a non-negative number.');
  }

  const overdue =
    resource.value !== null &&
    resource.updatedAt !== null &&
    now - resource.updatedAt > staleAfterMs;
  const state =
    resource.value === null
      ? 'loading'
      : resource.state === 'stale' || overdue
        ? 'stale'
        : resource.state;

  return {
    ...resource,
    state,
    refreshing: true,
    error: null,
    canRetry: false,
  };
}

export function resolveDesktopData<T>(
  resource: DesktopDataResource<T>,
  value: T,
  state: 'live' | 'mixed',
  source: string,
  now: number,
): DesktopDataResource<T> {
  requireActiveConnectedRefresh(resource);
  requireValue(value);
  if (state !== 'live' && state !== 'mixed') {
    throw new Error('A successful desktop data result must be live or mixed.');
  }
  return {
    ...resource,
    state,
    source,
    updatedAt: now,
    refreshing: false,
    error: null,
    canRetry: false,
    value,
  };
}

export function rejectDesktopData<T>(
  resource: DesktopDataResource<T>,
  error: string,
): DesktopDataResource<T> {
  requireActiveConnectedRefresh(resource);
  return {
    ...resource,
    state: resource.value === null ? 'unavailable' : 'stale',
    refreshing: false,
    error,
    canRetry: true,
  };
}

function requireValue<T>(value: T): asserts value is NonNullable<T> {
  if (value === null) {
    throw new Error('A ready desktop data resource requires a non-null value.');
  }
}

function requireConnected<T>(resource: DesktopDataResource<T>): void {
  if (resource.mode !== 'connected') {
    throw new Error('A live refresh requires a connected resource.');
  }
}

function requireActiveConnectedRefresh<T>(resource: DesktopDataResource<T>): void {
  requireValidResource(resource);
  requireConnected(resource);
  if (!resource.refreshing) {
    throw new Error('A desktop data result requires an active refresh.');
  }
}

function requireValidResource<T>(resource: DesktopDataResource<T>): void {
  if (
    (resource.state === 'live' || resource.state === 'mixed' || resource.state === 'stale') &&
    resource.value === null
  ) {
    throw new Error(`${resource.state} desktop data requires a non-null value.`);
  }
  if (resource.state === 'unavailable' && resource.value !== null) {
    throw new Error('Unavailable desktop data requires a null value.');
  }
  if (resource.mode === 'demo' && resource.state !== 'demo') {
    throw new Error('Demo mode requires demo resource state.');
  }
  if (resource.mode === 'connected' && resource.state === 'demo') {
    throw new Error('Connected mode cannot use demo resource state.');
  }
}
