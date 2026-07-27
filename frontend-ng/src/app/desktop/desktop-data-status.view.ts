// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
  output,
} from '@angular/core';
import type { DesktopDataResourceState, DesktopDataStatus } from './desktop-data-resource';

@Component({
  selector: 'lthn-desktop-data-status',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span class="desktop-data-status" aria-live="polite">
      <lthn-badge [attr.variant]="badgeVariant()" [attr.data-data-state]="status().state">
        {{ stateLabel() }}
      </lthn-badge>
      <span class="desktop-data-status__source">{{ status().source }}</span>
      @if (receivedAtLabel(); as receivedAt) {
        <span class="desktop-data-status__received">{{ receivedAt }}</span>
      }
      @if (status().refreshing) {
        <span class="desktop-data-status__refreshing" role="status">Refreshing</span>
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
      status.mode === 'connected' && status.canRetry && status.error !== null && !status.refreshing
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

function resourceStateVariant(state: DesktopDataResourceState): 'ok' | 'muted' | 'warn' {
  if (state === 'live') return 'ok';
  if (state === 'loading') return 'muted';
  return 'warn';
}
