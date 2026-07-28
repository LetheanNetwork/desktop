<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Desktop Data Resource and Telemetry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one truthful, immutable desktop-data lifecycle and make Telemetry
its first complete consumer, including deterministic demo mode, retained stale
data, retry, guarded polling, and teardown safety.

**Architecture:** Pure functions own resource transitions and Telemetry view
mapping. A presentation-only status component renders provenance and retry
state, while `TelemetryApp` remains the route-level coordinator that calls the
existing live-data service and owns its bounded timer.

**Tech Stack:** Angular 22 standalone components, signal inputs and outputs,
`OnPush`, TypeScript 6, Vitest/TestBed, Lit custom elements, npm, and the
existing Wails 3 bridge.

## Global Constraints

- Execute inline on `main`; do not use sub-agents or create a worktree.
- `frontend/` remains the only product frontend; do not add SSR,
  prerendering, hydration, or another frontend framework.
- Keep Angular 22 standalone components, `OnPush`, signals for local reactive
  state, and the existing Wails transport.
- Preserve Telemetry's route, window behaviour, two-panel layout, charts,
  visible copy, localisation coverage, custom elements, and five-second
  bounded poll cadence.
- Explicit offline transport must create deterministic, visibly labelled demo
  data without a Wails call, event subscription, or timer.
- Connected loading and first-failure states must not display fixture values.
- A failed refresh after a success must retain the complete last successful
  value and histories as stale data.
- A missing `wattsActive` value may use the explicitly labelled demo power
  panel and must resolve the overall resource to `mixed`.
- `updatedAt` is the local receipt time, not a backend sample timestamp.
- Map unknown failures to stable British-English reader copy; never render raw
  errors, stack traces, or transport payloads.
- Do not change Go services, generated bindings, NgRx, routes, design tokens,
  Control, Files, Terminal, or `SurfacePage` in this tranche.
- Use British English and retain EUPL-1.2 headers.
- Write each failing test before its production change.
- Leave the user-owned `go.work.sum` change and `.playwright-mcp/` directory
  untouched.

---

## File Map

- Create `frontend/src/app/desktop/desktop-data-resource.ts` for immutable
  generic resource types, invariants, and transitions.
- Create `frontend/src/app/desktop/desktop-data-resource.spec.ts` for the
  complete transition matrix.
- Create `frontend/src/app/desktop/desktop-data-status.view.ts` for
  provenance, receipt time, stale/unavailable copy, and retry presentation.
- Create `frontend/src/app/desktop/desktop-data-status.view.spec.ts` for the
  presenter's rendering and output contract.
- Create
  `frontend/src/app/desktop/apps/telemetry/telemetry-view.models.ts` for
  Telemetry-only readonly view types.
- Create
  `frontend/src/app/desktop/apps/telemetry/telemetry-view-state.ts` for
  deterministic demo mapping, live mapping, formatting, and bounded histories.
- Create
  `frontend/src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts` for
  pure view-state contracts.
- Modify `frontend/src/app/desktop/apps/telemetry.app.ts` so its only
  writable view state is
  `DesktopDataResource<TelemetryViewData>`.
- Modify `frontend/src/app/desktop/apps/telemetry.app.spec.ts` for demo,
  loading, live, mixed, unavailable, stale, retry, concurrency, polling, and
  destruction behaviour.
- Modify `frontend/src/app/desktop/desktop.component.scss` only to let the
  richer Telemetry metadata band wrap without losing its current visual
  hierarchy.

---

### Task 1: Immutable desktop-data resource transitions

**Files:**

- Create: `frontend/src/app/desktop/desktop-data-resource.ts`
- Test: `frontend/src/app/desktop/desktop-data-resource.spec.ts`

**Interfaces:**

- Consumes: no Angular service, bridge, clock, or timer.
- Produces:
  `DesktopDataResourceMode`,
  `DesktopDataResourceState`,
  `DesktopDataStatus`,
  `DesktopDataResource<T>`,
  `createDemoResource<T>(value, source)`,
  `createConnectedResource<T>(source)`,
  `beginDesktopDataRefresh<T>(resource, now, staleAfterMs)`,
  `resolveDesktopData<T>(resource, value, state, source, now)`, and
  `rejectDesktopData<T>(resource, error)`.

- [ ] **Step 1: Write the failing transition tests**

Create `desktop-data-resource.spec.ts`:

```ts
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

    expect(resolveDesktopData(
      refreshing,
      { value: 43 },
      'live',
      'Local process runtime',
      4_000,
    )).toMatchObject({
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

    expect(() => beginDesktopDataRefresh(demo, 1_000, 10_000)).toThrow(
      'connected resource',
    );
    expect(() =>
      resolveDesktopData(refreshingDemo, { value: 42 }, 'live', 'Live', 2_000),
    ).toThrow('connected resource');
    expect(() => rejectDesktopData(refreshingDemo, 'failed')).toThrow(
      'connected resource',
    );
    expect(() => beginDesktopDataRefresh(refreshing, 1_000, 10_000)).toThrow(
      'already refreshing',
    );
    expect(() =>
      resolveDesktopData(connected, { value: 1 }, 'live', 'Live', 1_000),
    ).toThrow('active refresh');
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
    expect(() =>
      resolveDesktopData(refreshing, null as never, 'live', 'Live', 2_000),
    ).toThrow('non-null value');
    expect(() => createDemoResource<Reading>(null as never, 'Demo')).toThrow(
      'non-null value',
    );
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

    expect(() =>
      beginDesktopDataRefresh(liveWithoutValue, 1_000, 10_000),
    ).toThrow('non-null value');
    expect(() =>
      beginDesktopDataRefresh(unavailableWithValue, 1_000, 10_000),
    ).toThrow('null value');
  });
});
```

- [ ] **Step 2: Run the spec and observe the missing-module failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-resource.spec.ts
```

Expected: FAIL because `desktop-data-resource.ts` does not exist.

- [ ] **Step 3: Implement the resource types and constructors**

Create `desktop-data-resource.ts` with the public model from the approved
design:

```ts
// SPDX-License-Identifier: EUPL-1.2

export type DesktopDataResourceMode = 'demo' | 'connected';

export type DesktopDataResourceState =
  | 'demo'
  | 'loading'
  | 'live'
  | 'mixed'
  | 'stale'
  | 'unavailable';

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

export function createDemoResource<T>(
  value: T,
  source: string,
): DesktopDataResource<T> {
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

export function createConnectedResource<T>(
  source: string,
): DesktopDataResource<T> {
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
```

- [ ] **Step 4: Implement guarded pure transitions**

Add the transition functions and private invariants to the same file:

```ts
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

function requireActiveConnectedRefresh<T>(
  resource: DesktopDataResource<T>,
): void {
  requireValidResource(resource);
  requireConnected(resource);
  if (!resource.refreshing) {
    throw new Error('A desktop data result requires an active refresh.');
  }
}

function requireValidResource<T>(resource: DesktopDataResource<T>): void {
  if (
    (resource.state === 'live' ||
      resource.state === 'mixed' ||
      resource.state === 'stale') &&
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
```

- [ ] **Step 5: Run the resource spec**

Run the Task 1 command again.

Expected: all resource tests PASS.

- [ ] **Step 6: Commit the resource contract**

```bash
git add frontend/src/app/desktop/desktop-data-resource.ts
git add frontend/src/app/desktop/desktop-data-resource.spec.ts
git commit -m "feat(frontend): add desktop data resource lifecycle"
```

---

### Task 2: Shared data-status presenter

**Files:**

- Create: `frontend/src/app/desktop/desktop-data-status.view.ts`
- Test: `frontend/src/app/desktop/desktop-data-status.view.spec.ts`

**Interfaces:**

- Consumes: `DesktopDataStatus`.
- Produces:
  `DesktopDataStatusView.status: InputSignal<DesktopDataStatus>` and
  `DesktopDataStatusView.retry: OutputEmitterRef<void>`.
- Does not receive a resource value and does not inject a service.

- [ ] **Step 1: Write failing presenter tests**

Create `desktop-data-status.view.spec.ts`:

```ts
import { TestBed } from '@angular/core/testing';
import type { DesktopDataStatus } from './desktop-data-resource';
import { DesktopDataStatusView } from './desktop-data-status.view';

describe('DesktopDataStatusView', () => {
  const live: DesktopDataStatus = {
    mode: 'connected',
    state: 'live',
    source: 'Local process runtime',
    updatedAt: new Date(2026, 6, 27, 12, 34, 56).getTime(),
    refreshing: false,
    error: null,
    canRetry: false,
  };

  function create(status: DesktopDataStatus) {
    const fixture = TestBed.createComponent(DesktopDataStatusView);
    fixture.componentRef.setInput('status', status);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => TestBed.resetTestingModule());

  it.each([
    ['demo', 'Demo data', 'warn'],
    ['loading', 'Loading live data', 'muted'],
    ['live', 'Live data', 'ok'],
    ['mixed', 'Live + demo', 'warn'],
    ['stale', 'Live data stale', 'warn'],
    ['unavailable', 'Live unavailable', 'warn'],
  ] as const)('renders the %s state label and established variant', (state, label, variant) => {
    const fixture = create({
      ...live,
      mode: state === 'demo' ? 'demo' : 'connected',
      state,
      updatedAt: null,
    });
    const badge = fixture.nativeElement.querySelector('lthn-badge');

    expect(badge.textContent).toContain(label);
    expect(badge.getAttribute('variant')).toBe(variant);
    expect(badge.dataset['dataState']).toBe(state);
  });

  it('shows reader-facing source and local receipt time', () => {
    const text = create(live).nativeElement.textContent;

    expect(text).toContain('Local process runtime');
    expect(text).toContain('Received 12:34:56');
  });

  it('shows refreshing without hiding the current live status', () => {
    const text = create({ ...live, refreshing: true }).nativeElement.textContent;

    expect(text).toContain('Live data');
    expect(text).toContain('Refreshing');
  });

  it('shows stale and unavailable explanations', () => {
    expect(
      create({
        ...live,
        state: 'stale',
        error: null,
      }).nativeElement.textContent,
    ).toContain('The last live reading is being retained.');
    expect(
      create({
        ...live,
        state: 'unavailable',
        updatedAt: null,
        error: 'Live telemetry is unavailable.',
      }).nativeElement.textContent,
    ).toContain('Live telemetry is unavailable.');
  });

  it('emits Retry only for a completed connected failure', () => {
    const fixture = create({
      ...live,
      state: 'stale',
      error: 'Refresh failed.',
      canRetry: true,
    });
    const emitted = vi.fn();
    fixture.componentInstance.retry.subscribe(emitted);

    fixture.nativeElement.querySelector('button[data-action="retry"]').click();

    expect(
      fixture.nativeElement
        .querySelector('button[data-action="retry"]')
        .getAttribute('aria-label'),
    ).toBe('Retry live data');
    expect(emitted).toHaveBeenCalledOnce();
  });

  it.each([
    { ...live, mode: 'demo' as const, state: 'demo' as const, canRetry: true },
    { ...live, state: 'stale' as const, canRetry: true, error: null },
    {
      ...live,
      state: 'stale' as const,
      canRetry: true,
      error: 'Refresh failed.',
      refreshing: true,
    },
  ])('does not expose Retry for invalid presentation state %#', (status) => {
    expect(create(status).nativeElement.querySelector('button')).toBeNull();
  });
});
```

- [ ] **Step 2: Run the spec and observe the missing-component failure**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-status.view.spec.ts
```

Expected: FAIL because `desktop-data-status.view.ts` does not exist.

- [ ] **Step 3: Implement the presentation-only component**

Create `desktop-data-status.view.ts`:

```ts
// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
  output,
} from '@angular/core';
import type {
  DesktopDataResourceState,
  DesktopDataStatus,
} from './desktop-data-resource';

@Component({
  selector: 'lthn-desktop-data-status',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span class="desktop-data-status" aria-live="polite">
      <lthn-badge
        [attr.variant]="badgeVariant()"
        [attr.data-data-state]="status().state"
      >
        {{ stateLabel() }}
      </lthn-badge>
      <span class="desktop-data-status__source">{{ status().source }}</span>
      @if (receivedAtLabel(); as receivedAt) {
        <span class="desktop-data-status__received">{{ receivedAt }}</span>
      }
      @if (status().refreshing) {
        <span class="desktop-data-status__refreshing" role="status"
          >Refreshing</span
        >
      }
      @if (detailLabel(); as detail) {
        <span class="desktop-data-status__detail">{{ detail }}</span>
      }
      @if (showRetry()) {
        <button
          type="button"
          class="desktop-data-status__retry"
          data-action="retry"
          aria-label="Retry live data"
          i18n-aria-label="Retry live data action@@desktop.data.retryAria"
          (click)="retry.emit()"
        >
          <span i18n="Retry live data action@@desktop.data.retry">Retry</span>
        </button>
      }
    </span>
  `,
  styles: `
    .desktop-data-status {
      display: inline-flex;
      align-items: center;
      flex-wrap: wrap;
      gap: 6px 9px;
      min-width: 0;
    }
    .desktop-data-status__source,
    .desktop-data-status__received,
    .desktop-data-status__refreshing,
    .desktop-data-status__detail {
      color: var(--fg-3);
      font-family: var(--font-mono);
      font-size: 10px;
    }
    .desktop-data-status__detail {
      color: var(--warning-fg, #febc2e);
    }
    .desktop-data-status__retry {
      border: 1px solid var(--line-2);
      border-radius: 6px;
      background: var(--ink-3);
      color: var(--fg-1);
      font: inherit;
      padding: 3px 8px;
      cursor: pointer;
    }
  `,
})
export class DesktopDataStatusView {
  readonly status = input.required<DesktopDataStatus>();
  readonly retry = output<void>();

  readonly stateLabel = computed(() => resourceStateLabel(this.status().state));
  readonly badgeVariant = computed(() => resourceStateVariant(this.status().state));
  readonly receivedAtLabel = computed(() => {
    const updatedAt = this.status().updatedAt;
    if (updatedAt === null) return null;
    const time = new Intl.DateTimeFormat('en-GB', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(new Date(updatedAt));
    return $localize`:Desktop data receipt time@@desktop.data.receivedAt:Received ${time}:time:`;
  });
  readonly detailLabel = computed(() => {
    const status = this.status();
    if (status.state === 'stale') {
      return (
        status.error ??
        $localize`:Stale desktop data explanation@@desktop.data.staleDetail:The last live reading is being retained.`
      );
    }
    if (status.state === 'unavailable') {
      return (
        status.error ??
        $localize`:Unavailable desktop data explanation@@desktop.data.unavailableDetail:Live data is unavailable.`
      );
    }
    return null;
  });
  readonly showRetry = computed(() => {
    const status = this.status();
    return (
      status.mode === 'connected' &&
      status.canRetry &&
      status.error !== null &&
      !status.refreshing
    );
  });
}

function resourceStateLabel(state: DesktopDataResourceState): string {
  switch (state) {
    case 'demo':
      return $localize`:Demo data state@@desktop.data.demo:Demo data`;
    case 'loading':
      return $localize`:Live data loading state@@desktop.data.loading:Loading live data`;
    case 'live':
      return $localize`:Live data state@@desktop.data.live:Live data`;
    case 'mixed':
      return $localize`:Mixed live and demo data state@@desktop.data.mixed:Live + demo`;
    case 'stale':
      return $localize`:Stale live data state@@desktop.data.stale:Live data stale`;
    case 'unavailable':
      return $localize`:Unavailable live data state@@desktop.data.resourceUnavailable:Live unavailable`;
  }
}

function resourceStateVariant(
  state: DesktopDataResourceState,
): 'ok' | 'muted' | 'warn' {
  if (state === 'live') return 'ok';
  if (state === 'loading') return 'muted';
  return 'warn';
}
```

Use `align-items: center`, not the British-English prose spelling, because CSS
property values are fixed language keywords.

- [ ] **Step 4: Run the presenter and compatibility badge specs**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-status.view.spec.ts \
  --include=src/app/desktop/desktop-data-state-badge.spec.ts
```

Expected: both specs PASS; the existing compatibility badge remains unchanged.

- [ ] **Step 5: Commit the status presenter**

```bash
git add frontend/src/app/desktop/desktop-data-status.view.ts
git add frontend/src/app/desktop/desktop-data-status.view.spec.ts
git commit -m "feat(frontend): add desktop data status presenter"
```

---

### Task 3: Pure Telemetry view state

**Files:**

- Create:
  `frontend/src/app/desktop/apps/telemetry/telemetry-view.models.ts`
- Create:
  `frontend/src/app/desktop/apps/telemetry/telemetry-view-state.ts`
- Test:
  `frontend/src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts`

**Interfaces:**

- Consumes: parsed `ProcessTelemetry` values and caller-supplied demo series.
- Produces:
  `TelemetryDemoSeries`,
  `TelemetryMetricView`,
  `TelemetryMetadataView`,
  `TelemetryViewData`,
  `TelemetryLiveViewResult`,
  `createDemoTelemetryView(series)`, and
  `createLiveTelemetryView(sample, previous, series)`.

- [ ] **Step 1: Write failing pure-mapping tests**

Create `telemetry-view-state.spec.ts`:

```ts
import type { ProcessTelemetry } from '../../desktop-live-data.service';
import {
  createDemoTelemetryView,
  createLiveTelemetryView,
} from './telemetry-view-state';
import type { TelemetryDemoSeries } from './telemetry-view.models';

const SERIES: TelemetryDemoSeries = {
  throughput: [28, 30, 46],
  watts: [180, 199, 210],
};

const SAMPLE: ProcessTelemetry = {
  heapAllocMB: 128.25,
  heapSysMB: 192.5,
  stackInUseMB: 4.75,
  numGoroutines: 42,
  numCgoCalls: 7,
  uptimeSeconds: 9_061,
  numGC: 18,
  lastGCPauseMs: 0.43,
  wattsActive: 0,
  wattsIdle: 0,
};

describe('Telemetry view state', () => {
  it('creates a fresh deterministic demo without sharing mutable series', () => {
    const first = createDemoTelemetryView(SERIES);
    const second = createDemoTelemetryView(SERIES);

    expect(first).toEqual(second);
    expect(first.sample).toBeNull();
    expect(first.primary).toMatchObject({
      label: 'Throughput',
      value: '41.8',
      unit: 'tok/s',
      provenance: 'demo',
      history: SERIES.throughput,
    });
    expect(first.power).toMatchObject({
      label: 'Power draw',
      value: '207',
      unit: 'W',
      provenance: 'demo',
      history: SERIES.watts,
    });
    expect(first.primary.history).not.toBe(SERIES.throughput);
    expect(first.metadata.map(({ label }) => label)).toEqual([
      'Model',
      'Region',
      'KV-cache',
      'Uptime',
    ]);
  });

  it('maps process data and labels absent native power as demo-backed mixed data', () => {
    const result = createLiveTelemetryView(SAMPLE, null, SERIES);

    expect(result.state).toBe('mixed');
    expect(result.value.sample).toBe(SAMPLE);
    expect(result.value.primary).toMatchObject({
      label: 'Heap allocation',
      value: '128.3',
      unit: 'MB',
      provenance: 'live',
      history: [128.25],
    });
    expect(result.value.power).toMatchObject({
      label: 'Power draw · demo',
      value: '207',
      unit: 'W',
      provenance: 'demo',
      history: SERIES.watts,
    });
    expect(result.value.metadata.map(({ value }) => value)).toEqual([
      '42',
      '0.43 ms',
      '7',
      '2h 31m',
    ]);
  });

  it('maps positive native power as live data', () => {
    const result = createLiveTelemetryView(
      { ...SAMPLE, wattsActive: 220.4 },
      null,
      SERIES,
    );

    expect(result.state).toBe('live');
    expect(result.value.power).toMatchObject({
      label: 'Power draw',
      value: '220',
      provenance: 'live',
      history: [220.4],
    });
  });

  it('appends and caps live histories without mutating the previous view', () => {
    const initial = createLiveTelemetryView(
      { ...SAMPLE, wattsActive: 200 },
      null,
      SERIES,
    ).value;
    const previous = {
      ...initial,
      primary: {
        ...initial.primary,
        history: Object.freeze(Array.from({ length: 60 }, (_, index) => index)),
      },
      power: {
        ...initial.power,
        history: Object.freeze(Array.from({ length: 60 }, (_, index) => 100 + index)),
      },
    };

    const result = createLiveTelemetryView(
      { ...SAMPLE, heapAllocMB: 999, wattsActive: 250 },
      previous,
      SERIES,
    );

    expect(result.value.primary.history).toHaveLength(60);
    expect(result.value.primary.history[0]).toBe(1);
    expect(result.value.primary.history.at(-1)).toBe(999);
    expect(result.value.power.history).toHaveLength(60);
    expect(result.value.power.history[0]).toBe(101);
    expect(result.value.power.history.at(-1)).toBe(250);
    expect(previous.primary.history[0]).toBe(0);
    expect(previous.power.history[0]).toBe(100);
  });

  it('does not carry demo chart points into the first live history', () => {
    const demo = createDemoTelemetryView(SERIES);
    const result = createLiveTelemetryView(
      { ...SAMPLE, wattsActive: 220 },
      demo,
      SERIES,
    );

    expect(result.value.primary.history).toEqual([128.25]);
    expect(result.value.power.history).toEqual([220]);
  });
});
```

- [ ] **Step 2: Run the spec and observe the missing-module failures**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts
```

Expected: FAIL because the Telemetry view modules do not exist.

- [ ] **Step 3: Define the readonly Telemetry view types**

Create `telemetry-view.models.ts`:

```ts
// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';

export type TelemetryPanelProvenance = 'demo' | 'live';
export type TelemetryConnectedState = 'live' | 'mixed';

export interface TelemetryDemoSeries {
  readonly throughput: readonly number[];
  readonly watts: readonly number[];
}

export interface TelemetryMetricView {
  readonly label: string;
  readonly value: string;
  readonly unit: string;
  readonly history: readonly number[];
  readonly provenance: TelemetryPanelProvenance;
}

export interface TelemetryMetadataView {
  readonly label: string;
  readonly value: string;
}

export interface TelemetryViewData {
  readonly sample: ProcessTelemetry | null;
  readonly primary: TelemetryMetricView;
  readonly power: TelemetryMetricView;
  readonly metadata: readonly TelemetryMetadataView[];
}

export interface TelemetryLiveViewResult {
  readonly state: TelemetryConnectedState;
  readonly value: TelemetryViewData;
}
```

- [ ] **Step 4: Implement deterministic demo and live mapping**

Create `telemetry-view-state.ts`:

```ts
// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';
import type {
  TelemetryDemoSeries,
  TelemetryLiveViewResult,
  TelemetryViewData,
} from './telemetry-view.models';

const MAX_HISTORY_SAMPLES = 60;

export function createDemoTelemetryView(
  series: TelemetryDemoSeries,
): TelemetryViewData {
  return {
    sample: null,
    primary: {
      label: $localize`:Telemetry metric@@telemetry.throughput:Throughput`,
      value: '41.8',
      unit: $localize`:Tokens per second unit@@unit.tokensPerSecondInline:tok/s`,
      history: [...series.throughput],
      provenance: 'demo',
    },
    power: {
      label: $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
      value: '207',
      unit: $localize`:Watts unit@@unit.watts:W`,
      history: [...series.watts],
      provenance: 'demo',
    },
    metadata: [
      {
        label: $localize`:Telemetry model label@@telemetry.modelLabel:Model`,
        value: 'llama-3.1-70b',
      },
      {
        label: $localize`:Telemetry region label@@telemetry.regionLabel:Region`,
        value: 'eu-west-2',
      },
      {
        label: $localize`:Telemetry cache label@@telemetry.kvCacheLabel:KV-cache`,
        value: '62%',
      },
      {
        label: $localize`:Telemetry uptime label@@telemetry.uptimeLabel:Uptime`,
        value: '6d 4h',
      },
    ],
  };
}

export function createLiveTelemetryView(
  sample: ProcessTelemetry,
  previous: TelemetryViewData | null,
  demoSeries: TelemetryDemoSeries,
): TelemetryLiveViewResult {
  const previousLive = previous?.sample === null ? null : previous;
  const hasLivePower = sample.wattsActive > 0;
  const value: TelemetryViewData = {
    sample,
    primary: {
      label: $localize`:Process heap telemetry metric@@telemetry.heapAllocation:Heap allocation`,
      value: sample.heapAllocMB.toFixed(1),
      unit: $localize`:Megabytes unit@@unit.megabytes:MB`,
      history: appendHistory(previousLive?.primary.history ?? [], sample.heapAllocMB),
      provenance: 'live',
    },
    power: hasLivePower
      ? {
          label: $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
          value: sample.wattsActive.toFixed(0),
          unit: $localize`:Watts unit@@unit.watts:W`,
          history: appendHistory(
            previousLive?.power.provenance === 'live'
              ? previousLive.power.history
              : [],
            sample.wattsActive,
          ),
          provenance: 'live',
        }
      : {
          label: $localize`:Demo power telemetry metric@@telemetry.demoPowerDraw:Power draw · demo`,
          value: '207',
          unit: $localize`:Watts unit@@unit.watts:W`,
          history: [...demoSeries.watts],
          provenance: 'demo',
        },
    metadata: [
      {
        label: $localize`:Telemetry goroutines label@@telemetry.goroutines:Goroutines`,
        value: String(sample.numGoroutines),
      },
      {
        label: $localize`:Telemetry GC pause label@@telemetry.gcPause:GC pause`,
        value: `${sample.lastGCPauseMs} ms`,
      },
      {
        label: $localize`:Telemetry CGO calls label@@telemetry.cgoCalls:CGO calls`,
        value: String(sample.numCgoCalls),
      },
      {
        label: $localize`:Telemetry uptime label@@telemetry.uptimeLabel:Uptime`,
        value: formatUptime(sample.uptimeSeconds),
      },
    ],
  };

  return { state: hasLivePower ? 'live' : 'mixed', value };
}

function appendHistory(
  history: readonly number[],
  value: number,
): readonly number[] {
  return [...history, value].slice(-MAX_HISTORY_SAMPLES);
}

function formatUptime(seconds: number): string {
  const wholeSeconds = Math.max(0, Math.floor(seconds));
  const days = Math.floor(wholeSeconds / 86_400);
  const hours = Math.floor((wholeSeconds % 86_400) / 3_600);
  const minutes = Math.floor((wholeSeconds % 3_600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}
```

- [ ] **Step 5: Run the Telemetry mapping spec**

Run the Task 3 command again.

Expected: all mapping tests PASS.

- [ ] **Step 6: Commit the Telemetry view state**

```bash
git add frontend/src/app/desktop/apps/telemetry/telemetry-view.models.ts
git add frontend/src/app/desktop/apps/telemetry/telemetry-view-state.ts
git add frontend/src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts
git commit -m "refactor(frontend): model Telemetry view state"
```

---

### Task 4: Adopt the resource lifecycle in Telemetry

**Files:**

- Modify: `frontend/src/app/desktop/apps/telemetry.app.ts`
- Modify: `frontend/src/app/desktop/apps/telemetry.app.spec.ts`
- Modify: `frontend/src/app/desktop/desktop.component.scss:71-79`

**Interfaces:**

- Consumes:
  `DesktopLiveDataService.mode`,
  `DesktopLiveDataService.telemetry()`,
  all Task 1 resource transitions,
  `DesktopDataStatusView`, and
  both Task 3 mapping functions.
- Produces:
  `TelemetryApp.resource:
  WritableSignal<DesktopDataResource<TelemetryViewData>>` and the existing
  public `refresh(): Promise<void>` retry entrypoint.

- [ ] **Step 1: Replace the old component tests with failing lifecycle tests**

Retain `telemetryWin`, the TestBed provider, and a reusable parsed sample. Use
this structure in `telemetry.app.spec.ts`:

```ts
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import type { ProcessTelemetry } from '../desktop-live-data.service';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import type { Win } from '../desktop.data';
import { TelemetryApp } from './telemetry.app';

const SAMPLE: ProcessTelemetry = {
  heapAllocMB: 128.25,
  heapSysMB: 192.5,
  stackInUseMB: 4.75,
  numGoroutines: 42,
  numCgoCalls: 7,
  uptimeSeconds: 9_061,
  numGC: 18,
  lastGCPauseMs: 0.43,
  wattsActive: 0,
  wattsIdle: 0,
};

describe('TelemetryApp', () => {
  const mode = signal<'demo' | 'live'>('demo');
  const liveData = {
    mode: mode.asReadonly(),
    telemetry: vi.fn(),
  };

  beforeEach(() => {
    mode.set('demo');
    liveData.telemetry.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: DesktopLiveDataService, useValue: liveData }],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    TestBed.resetTestingModule();
  });

  function create() {
    const fixture = TestBed.createComponent(TelemetryApp);
    fixture.componentRef.setInput('win', telemetryWin);
    fixture.detectChanges();
    return fixture;
  }

  it('renders deterministic labelled demo data without a live read', () => {
    const fixture = create();
    const text = fixture.nativeElement.textContent;

    expect(fixture.componentInstance.resource().state).toBe('demo');
    expect(text).toContain('Demo data');
    expect(text).toContain('Lethean demo fixture');
    expect(text).toContain('41.8');
    expect(text).toContain('llama-3.1-70b');
    expect(liveData.telemetry).not.toHaveBeenCalled();
  });

  it('starts connected with loading placeholders and no fixture substitution', () => {
    mode.set('live');
    liveData.telemetry.mockReturnValue(new Promise(() => undefined));
    const fixture = create();
    const text = fixture.nativeElement.textContent;

    expect(fixture.componentInstance.resource()).toMatchObject({
      mode: 'connected',
      state: 'loading',
      refreshing: true,
      value: null,
    });
    expect(text).toContain('Loading live data');
    expect(text).not.toContain('41.8');
    expect(text).not.toContain('207');
    expect(text).not.toContain('llama-3.1-70b');
  });

  it('renders live process data with explicitly demo-backed power', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue(SAMPLE);
    const fixture = create();

    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('mixed'),
    );
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent;

    expect(text).toContain('Live + demo');
    expect(text).toContain('Local process runtime');
    expect(text).toContain('Heap allocation');
    expect(text).toContain('128.3');
    expect(text).toContain('Power draw · demo');
    expect(text).toContain('Goroutines 42');
    expect(text).toContain('Uptime 2h 31m');
    expect(fixture.componentInstance.resource().updatedAt).not.toBeNull();
  });

  it('renders positive native power as wholly live', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValue({ ...SAMPLE, wattsActive: 220.4 });
    const fixture = create();

    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('live'),
    );
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Live data');
    expect(fixture.nativeElement.textContent).toContain('220');
    expect(fixture.nativeElement.textContent).not.toContain('Power draw · demo');
  });

  it('makes a first failure unavailable without showing fixtures or raw errors', async () => {
    mode.set('live');
    liveData.telemetry.mockRejectedValue(new Error('socket secret detail'));
    const fixture = create();

    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('unavailable'),
    );
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent;

    expect(text).toContain('Live telemetry is unavailable.');
    expect(text).not.toContain('socket secret detail');
    expect(text).not.toContain('41.8');
    expect(text).not.toContain('207');
    expect(fixture.componentInstance.resource().value).toBeNull();
  });

  it('retains the last successful sample and charts as stale after a later failure', async () => {
    mode.set('live');
    liveData.telemetry.mockResolvedValueOnce(SAMPLE);
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('mixed'),
    );
    const successful = fixture.componentInstance.resource();

    liveData.telemetry.mockRejectedValueOnce(new Error('connection dropped'));
    await fixture.componentInstance.refresh();
    fixture.detectChanges();

    expect(fixture.componentInstance.resource()).toMatchObject({
      state: 'stale',
      updatedAt: successful.updatedAt,
      value: successful.value,
      canRetry: true,
    });
    expect(fixture.nativeElement.textContent).toContain('128.3');
    expect(fixture.nativeElement.textContent).toContain('Live telemetry is unavailable.');
  });

  it('routes the status Retry action through the same refresh path', async () => {
    mode.set('live');
    liveData.telemetry.mockRejectedValueOnce(new Error('offline'));
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('unavailable'),
    );
    liveData.telemetry.mockResolvedValueOnce(SAMPLE);
    fixture.detectChanges();

    fixture.nativeElement.querySelector('button[data-action="retry"]').click();

    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('mixed'),
    );
    expect(liveData.telemetry).toHaveBeenCalledTimes(2);
  });

  it('recovers stale data without resetting its successful history', async () => {
    mode.set('live');
    liveData.telemetry
      .mockResolvedValueOnce(SAMPLE)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ ...SAMPLE, heapAllocMB: 130 });
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('mixed'),
    );

    await fixture.componentInstance.refresh();
    expect(fixture.componentInstance.resource().state).toBe('stale');
    await fixture.componentInstance.refresh();

    expect(fixture.componentInstance.resource()).toMatchObject({
      state: 'mixed',
      error: null,
      canRetry: false,
    });
    expect(
      fixture.componentInstance.resource().value?.primary.history,
    ).toEqual([128.25, 130]);
  });

  it('skips an overlapping manual refresh', async () => {
    mode.set('live');
    let resolve!: (sample: ProcessTelemetry) => void;
    liveData.telemetry.mockReturnValue(
      new Promise<ProcessTelemetry>((accept) => {
        resolve = accept;
      }),
    );
    const fixture = create();

    void fixture.componentInstance.refresh();
    expect(liveData.telemetry).toHaveBeenCalledOnce();

    resolve(SAMPLE);
    await vi.waitFor(() =>
      expect(fixture.componentInstance.resource().state).toBe('mixed'),
    );
  });
});
```

- [ ] **Step 2: Run the app spec and observe failures against the old signals**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/telemetry.app.spec.ts
```

Expected: FAIL because `TelemetryApp.resource` and the new status lifecycle do
not exist.

- [ ] **Step 3: Replace separate writable signals with one resource**

In `telemetry.app.ts`, remove `sample`, `dataState`, `heapHistory`, and
`powerHistory`. Keep the existing inputs and introduce:

```ts
const TELEMETRY_POLL_MS = 5_000;
const TELEMETRY_STALE_AFTER_MS = TELEMETRY_POLL_MS * 2;
const TELEMETRY_DEMO_SOURCE =
  $localize`:Telemetry demo source@@telemetry.source.demo:Lethean demo fixture`;
const TELEMETRY_LIVE_SOURCE =
  $localize`:Telemetry live source@@telemetry.source.live:Local process runtime`;
const TELEMETRY_UNAVAILABLE =
  $localize`:Telemetry unavailable error@@telemetry.data.unavailable:Live telemetry is unavailable.`;

@Input() win!: Win;
@Input() throughput: readonly number[] = TELEMETRY.throughput;
@Input() watts: readonly number[] = TELEMETRY.watts;

readonly resource = signal<DesktopDataResource<TelemetryViewData>>(
  createDemoResource(
    createDemoTelemetryView({
      throughput: this.throughput,
      watts: this.watts,
    }),
    TELEMETRY_DEMO_SOURCE,
  ),
);
readonly view = computed(() => this.resource().value);
readonly primaryLabel = computed(
  () =>
    this.view()?.primary.label ??
    $localize`:Process heap telemetry metric@@telemetry.heapAllocation:Heap allocation`,
);
readonly primaryValue = computed(() => this.view()?.primary.value ?? '—');
readonly primaryUnit = computed(() => this.view()?.primary.unit ?? '');
readonly powerLabel = computed(
  () =>
    this.view()?.power.label ??
    $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
);
readonly powerValue = computed(() => this.view()?.power.value ?? '—');
readonly powerUnit = computed(() => this.view()?.power.unit ?? '');
readonly metadata = computed(() => this.view()?.metadata ?? []);
readonly primaryJson = computed(() =>
  JSON.stringify(this.view()?.primary.history ?? []),
);
readonly powerJson = computed(() =>
  JSON.stringify(this.view()?.power.history ?? []),
);
```

Import the exact dependencies:

```ts
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from '../desktop-data-resource';
import { DesktopDataStatusView } from '../desktop-data-status.view';
import type { TelemetryViewData } from './telemetry/telemetry-view.models';
import {
  createDemoTelemetryView,
  createLiveTelemetryView,
} from './telemetry/telemetry-view-state';
```

Replace `DesktopDataStateBadge` with `DesktopDataStatusView` in the standalone
component imports. Remove `CommonModule`, `DesktopDataState`,
`DesktopDataStateBadge`, and the direct `ProcessTelemetry` type import after
the old structural-template and signal code is gone:

```ts
imports: [DesktopDataStatusView],
```

Use this class declaration for the timer-free Task 4 intermediate state:

```ts
export class TelemetryApp implements AppView, OnInit {
```

Update the file header to describe deterministic offline demo data, retained
stale values, and guarded connected refreshes. Remove its obsolete claim that
unavailable connected data falls back to the demo composition.

- [ ] **Step 4: Implement initialisation and guarded refresh**

Add a request guard. Initialise from the final Angular inputs during
`ngOnInit`, so input overrides remain supported:

```ts
private refreshInFlight = false;

ngOnInit(): void {
  const demo = createDemoTelemetryView(this.demoSeries());
  if (this.liveData.mode() === 'demo') {
    this.resource.set(createDemoResource(demo, TELEMETRY_DEMO_SOURCE));
    return;
  }

  this.resource.set(
    createConnectedResource<TelemetryViewData>(TELEMETRY_LIVE_SOURCE),
  );
  void this.refresh();
}

async refresh(): Promise<void> {
  if (this.liveData.mode() === 'demo' || this.refreshInFlight) return;
  this.refreshInFlight = true;
  this.resource.update((resource) =>
    beginDesktopDataRefresh(
      resource,
      Date.now(),
      TELEMETRY_STALE_AFTER_MS,
    ),
  );

  try {
    const sample = await this.liveData.telemetry();
    const mapped = createLiveTelemetryView(
      sample,
      this.resource().value,
      this.demoSeries(),
    );
    this.resource.update((resource) =>
      resolveDesktopData(
        resource,
        mapped.value,
        mapped.state,
        TELEMETRY_LIVE_SOURCE,
        Date.now(),
      ),
    );
  } catch {
    this.resource.update((resource) =>
      rejectDesktopData(resource, TELEMETRY_UNAVAILABLE),
    );
  } finally {
    this.refreshInFlight = false;
  }
}

private demoSeries() {
  return {
    throughput: this.throughput,
    watts: this.watts,
  };
}
```

Do not add the interval yet; Task 5 introduces and tests scheduling, timer
cleanup, and late-result protection together.

Remove the old `pollHandle`, timer setup, `ngOnDestroy`, `OnDestroy` import, and
`OnDestroy` class interface in this intermediate commit. Task 5 restores them
with their behavioural tests.

- [ ] **Step 5: Bind the existing layout to the view model and status presenter**

Keep the two `.big` panels and their existing glow/sparkline attributes.
Replace their dynamic bindings and the metadata band with:

```html
<span class="lab">{{ primaryLabel() }}</span>
<div class="num">
  {{ primaryValue() }}<small> {{ primaryUnit() }}</small>
</div>
<lthn-sparkline
  [attr.data]="primaryJson()"
  color="var(--brand-300)"
  width="260"
  height="46"
  fill
></lthn-sparkline>
```

```html
<span class="lab">{{ powerLabel() }}</span>
<div class="num">
  {{ powerValue() }}<small> {{ powerUnit() }}</small>
</div>
<lthn-sparkline
  [attr.data]="powerJson()"
  color="#febc2e"
  width="260"
  height="46"
  fill
></lthn-sparkline>
```

```html
<div class="metaband">
  <lthn-desktop-data-status
    [status]="resource()"
    (retry)="refresh()"
  />
  @for (item of metadata(); track item.label) {
    <span>{{ item.label }} <b>{{ item.value }}</b></span>
  }
</div>
```

The em dash and empty `[]` series are the connected loading/unavailable
placeholders. Do not introduce a fixture fallback when `resource().value` is
null.

In `desktop.component.scss`, preserve the existing values and add wrapping:

```scss
.metaband {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px 26px;
  padding: 14px 24px;
  border-top: 1px solid var(--line-1);
  background: var(--ink-2);
  font-size: 12px;
  color: var(--fg-2);
}
```

- [ ] **Step 6: Run focused lifecycle tests**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-resource.spec.ts \
  --include=src/app/desktop/desktop-data-status.view.spec.ts \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts
```

Expected: all focused specs PASS.

- [ ] **Step 7: Commit the Telemetry adoption**

```bash
git add frontend/src/app/desktop/apps/telemetry.app.ts
git add frontend/src/app/desktop/apps/telemetry.app.spec.ts
git add frontend/src/app/desktop/desktop.component.scss
git commit -m "feat(frontend): adopt data lifecycle in Telemetry"
```

---

### Task 5: Polling cadence and destruction safety

**Files:**

- Modify: `frontend/src/app/desktop/apps/telemetry.app.ts`
- Modify: `frontend/src/app/desktop/apps/telemetry.app.spec.ts`

**Interfaces:**

- Consumes: Task 4's guarded `refresh()` path.
- Produces: one five-second connected timer, no demo timer, skipped overlapping
  polls, timer cleanup, and ignored late settlements after destruction.

- [ ] **Step 1: Add failing timer and late-result tests**

Append these cases to `telemetry.app.spec.ts`:

```ts
it('polls every five seconds and stops polling when destroyed', async () => {
  vi.useFakeTimers();
  mode.set('live');
  liveData.telemetry.mockResolvedValue(SAMPLE);
  const fixture = create();

  await vi.advanceTimersByTimeAsync(0);
  expect(liveData.telemetry).toHaveBeenCalledOnce();

  liveData.telemetry.mockClear();
  await vi.advanceTimersByTimeAsync(4_999);
  expect(liveData.telemetry).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(liveData.telemetry).toHaveBeenCalledOnce();

  fixture.destroy();
  liveData.telemetry.mockClear();
  await vi.advanceTimersByTimeAsync(5_000);
  expect(liveData.telemetry).not.toHaveBeenCalled();
});

it('does not create a polling timer in demo mode', () => {
  vi.useFakeTimers();
  const setInterval = vi.spyOn(window, 'setInterval');
  const fixture = create();

  vi.advanceTimersByTime(15_000);

  expect(setInterval).not.toHaveBeenCalled();
  expect(liveData.telemetry).not.toHaveBeenCalled();
  fixture.destroy();
});

it('ignores a live result that settles after destruction', async () => {
  mode.set('live');
  let resolve!: (sample: ProcessTelemetry) => void;
  liveData.telemetry.mockReturnValue(
    new Promise<ProcessTelemetry>((accept) => {
      resolve = accept;
    }),
  );
  const fixture = create();
  const resourceAtDestroy = fixture.componentInstance.resource();

  fixture.destroy();
  resolve(SAMPLE);
  await Promise.resolve();
  await Promise.resolve();

  expect(fixture.componentInstance.resource()).toBe(resourceAtDestroy);
  expect(fixture.componentInstance.resource().value).toBeNull();
});
```

- [ ] **Step 2: Run the app spec and observe cadence/teardown failures**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/apps/telemetry.app.spec.ts
```

Expected: FAIL because no interval exists and a late settlement still updates
the resource.

- [ ] **Step 3: Add the bounded timer and destruction guard**

Add:

```ts
private pollHandle: number | undefined;
private destroyed = false;
```

Restore the `OnDestroy` import and
`implements AppView, OnInit, OnDestroy` class interface in this task.

After `void this.refresh()` in the connected `ngOnInit` branch:

```ts
this.pollHandle = window.setInterval(
  () => void this.refresh(),
  TELEMETRY_POLL_MS,
);
```

Implement teardown:

```ts
ngOnDestroy(): void {
  this.destroyed = true;
  if (this.pollHandle !== undefined) {
    window.clearInterval(this.pollHandle);
    this.pollHandle = undefined;
  }
}
```

Extend the refresh guard and both settlement paths:

```ts
if (
  this.liveData.mode() === 'demo' ||
  this.refreshInFlight ||
  this.destroyed
) {
  return;
}
```

Immediately after `await this.liveData.telemetry()`:

```ts
if (this.destroyed) return;
```

At the start of `catch`:

```ts
if (this.destroyed) return;
```

The `finally` block may still clear `refreshInFlight`; it is non-rendered
internal state and does not mutate the destroyed Angular resource.

- [ ] **Step 4: Run the complete focused set**

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-data-resource.spec.ts \
  --include=src/app/desktop/desktop-data-status.view.spec.ts \
  --include=src/app/desktop/apps/telemetry/telemetry-view-state.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts
```

Expected: all focused specs PASS, including cadence and late-result cleanup.

- [ ] **Step 5: Commit lifecycle hardening**

```bash
git add frontend/src/app/desktop/apps/telemetry.app.ts
git add frontend/src/app/desktop/apps/telemetry.app.spec.ts
git commit -m "test(frontend): harden Telemetry polling lifecycle"
```

---

## Final Verification

- [ ] **Step 1: Run the deterministic frontend confidence gate**

From the repository root:

```bash
go tool wails3 task verify:frontend
```

Expected ordered results:

- capability inventory succeeds;
- frontend convergence contracts pass;
- Angular CI tests pass;
- the Angular production build succeeds;
- embedded frontend output verification succeeds.

- [ ] **Step 2: Verify formatting and exact working-tree scope**

```bash
git diff --check
git status --short
git log -5 --oneline
```

Expected:

- `git diff --check` emits no output;
- only the pre-existing user-owned `go.work.sum` and `.playwright-mcp/`
  changes remain outside the five implementation commits;
- the five task commits are visible in order.

- [ ] **Step 3: Inspect the final implementation against the acceptance contract**

Confirm directly from the focused tests and rendered DOM:

- offline Telemetry is deterministic, labelled, Wails-free, and timer-free;
- connected loading and initial failure show no fixture numbers;
- success is live or mixed according to power provenance;
- a later failure keeps the last sample and both histories as stale;
- source, local receipt time, refresh progress, reader-safe error, and Retry
  are visible in the correct states;
- polling never overlaps, stops on destruction, and ignores late results;
- Control, Files, Terminal, `SurfacePage`, Go, bindings, routing, and NgRx are
  unchanged.
