// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
  output,
} from '@angular/core';
import { Store } from '@ngrx/store';
import { ConnectionManagerService } from '../connection-manager.service';
import { desktopControlsActions } from '../store/desktop-controls.actions';
import type { DesktopControl, DesktopControlValue } from '../store/desktop-controls.models';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectHasDirtyDesktopControls,
} from '../store/desktop-controls.reducer';
import { DesktopDataStateBadge } from './desktop-data-state-badge';
import type { DesktopDataState } from './desktop-data-state';
import type { DesktopPermissionID, DesktopPermissionSnapshot } from './desktop-host-intent.service';

const PERMISSION_CONTROL_IDS: Readonly<Record<string, DesktopPermissionID>> = {
  'desktop.permissions.microphone': 'microphone',
  'desktop.permissions.camera': 'camera',
  'desktop.permissions.geolocation': 'geolocation',
  'desktop.permissions.notifications': 'notifications',
  'desktop.permissions.clipboard_read': 'clipboard-read',
};

@Component({
  selector: 'lthn-desktop-controls-panel',
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="desktop-controls-panel" [attr.aria-label]="heading()">
      <div class="controls-heading">
        <div>
          <h1>{{ heading() }}</h1>
          <p>{{ description() }}</p>
        </div>
        <div class="controls-context">
          <lthn-desktop-data-state [state]="dataState()" />
          @if (precedence()) {
            <span class="cfgsrc">{{ precedence() }}</span>
          }
        </div>
      </div>

      @if (showActions()) {
        <div class="settings-actions" aria-label="Settings draft actions">
          <button
            type="button"
            data-action="reset-settings"
            [disabled]="loading() || saving()"
            (click)="resetDraft()"
          >
            Reset
          </button>
          <button
            type="button"
            data-action="discard-settings"
            [disabled]="!hasDraftChanges() || saving()"
            (click)="discardDraft()"
          >
            Discard
          </button>
          <button
            type="button"
            class="primary"
            data-action="apply-settings"
            [disabled]="!hasDraftChanges() || saving()"
            (click)="applyDraft()"
          >
            {{ saving() ? 'Applying…' : 'Apply' }}
          </button>
        </div>
      }

      @if (showRestartSummary() && restartSummary()) {
        <p class="controls-restart-summary" role="status">{{ restartSummary() }}</p>
      }
      @if (error()) {
        <div class="controls-error" role="alert">
          <span>{{ error() }}</span>
          <button
            type="button"
            data-action="retry-settings"
            [disabled]="loading() || saving()"
            (click)="retry()"
          >
            Retry
          </button>
        </div>
      }
      @if (loading() && groups().length === 0) {
        <p class="controls-status" role="status">Loading desktop controls…</p>
      }

      @for (group of visibleGroups(); track group.name) {
        <div class="setgroup control-group">
          <span class="glab">{{ group.name }}</span>
          @for (control of group.controls; track control.key) {
            <div
              class="setrow desktop-control"
              [attr.data-control-key]="control.key"
              [class.is-saving]="saving()"
            >
              <div class="k">
                {{ control.label }}
                <small>{{ control.description }}</small>
                <code class="control-key">{{ control.key }}</code>
                <span class="control-badges">
                  @if (control.live) {
                    <span class="control-badge live">Live</span>
                  }
                  @if (control.restartRequired) {
                    <span class="control-badge restart">Restart required</span>
                  }
                  @if (control.configured) {
                    <span class="control-badge configured">Custom</span>
                  }
                  @if (saving()) {
                    <span class="control-badge saving">Saving…</span>
                  }
                  @if (permissionForControl(control.key); as permission) {
                    <span class="control-badge host">Host: {{ permission.host }}</span>
                  }
                </span>
              </div>

              <div class="control-actions">
                @switch (control.kind) {
                  @case ('toggle') {
                    <div class="prefseg" role="group" [attr.aria-label]="control.label">
                      <button
                        type="button"
                        data-value="true"
                        [class.on]="control.value === true"
                        [disabled]="saving()"
                        (click)="setControl(control, true)"
                      >
                        On
                      </button>
                      <button
                        type="button"
                        data-value="false"
                        [class.on]="control.value === false"
                        [disabled]="saving()"
                        (click)="setControl(control, false)"
                      >
                        Off
                      </button>
                    </div>
                  }
                  @case ('select') {
                    <select
                      class="cfgin control-input"
                      [value]="control.value"
                      [disabled]="saving()"
                      [attr.aria-label]="control.label"
                      (change)="setChoice(control, $event)"
                    >
                      @for (choice of control.choices ?? []; track choice) {
                        <option [value]="choice">{{ choice }}</option>
                      }
                    </select>
                  }
                  @case ('number') {
                    <input
                      class="cfgin control-input"
                      type="number"
                      [value]="control.value"
                      [attr.min]="control.minimum ?? null"
                      [attr.max]="control.maximum ?? null"
                      [attr.step]="control.step ?? null"
                      [disabled]="saving()"
                      [attr.aria-label]="control.label"
                      (change)="setNumber(control, $event)"
                    />
                  }
                  @default {
                    <input
                      class="cfgin control-input"
                      type="text"
                      [value]="control.value"
                      [disabled]="saving()"
                      [attr.aria-label]="control.label"
                      (change)="setText(control, $event)"
                    />
                  }
                }
                @if (canRequestPermission(control.key)) {
                  <button
                    type="button"
                    class="permission-request"
                    data-action="request-permission-notifications"
                    [disabled]="requestingPermission() !== null"
                    (click)="requestPermission(control.key)"
                  >
                    {{
                      requestingPermission() === 'notifications'
                        ? 'Requesting…'
                        : 'Request host access'
                    }}
                  </button>
                }
              </div>
            </div>
          }
        </div>
      }
      @if (help()) {
        <p class="cfghint">
          <lthn-icon name="circle-info" size="11"></lthn-icon>
          {{ help() }}
        </p>
      }
    </section>
  `,
  styles: `
    .desktop-controls-panel {
      display: block;
      margin-top: 22px;
      padding-bottom: 24px;
    }
    .controls-heading {
      display: flex;
      align-items: flex-end;
      justify-content: space-between;
      gap: 16px;
      margin: 0 16px 12px;
    }
    .controls-heading h1 {
      margin: 0;
      font-size: 16px;
    }
    .controls-heading p,
    .controls-status {
      margin: 4px 16px 12px;
      color: var(--text-muted, #8d929b);
      font-size: 11px;
    }
    .controls-heading p {
      margin: 4px 0 0;
    }
    .controls-context {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      justify-content: flex-end;
      gap: 8px;
    }
    .cfgsrc,
    .control-key {
      color: var(--fg-3, #8d929b);
      font-family: var(--font-mono);
      font-size: 10px;
    }
    .settings-actions {
      display: flex;
      justify-content: flex-end;
      gap: 7px;
      margin: 0 16px 12px;
    }
    .settings-actions button,
    .controls-error button,
    .permission-request {
      min-width: 72px;
      padding: 6px 10px;
      border: 1px solid var(--border, #30343b);
      border-radius: 6px;
      background: var(--surface-2, #181a20);
      color: var(--text, #e7e9ed);
      cursor: pointer;
    }
    .settings-actions button.primary {
      border-color: color-mix(in srgb, var(--brand-500, #8f56c2) 70%, transparent);
      background: var(--brand-600, #73419f);
    }
    button:disabled {
      cursor: default;
      opacity: 0.48;
    }
    .controls-error {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin: 0 16px 12px;
      padding: 8px 10px;
      border: 1px solid color-mix(in srgb, #ef6b73 45%, transparent);
      border-radius: 7px;
      background: color-mix(in srgb, #ef6b73 10%, transparent);
      color: #ef9aa0;
      font-size: 11px;
    }
    .controls-error button {
      min-width: auto;
      padding: 4px 8px;
    }
    .controls-restart-summary {
      margin: 0 16px 12px;
      color: #e3b35b;
      font-size: 11px;
    }
    .control-group {
      margin-bottom: 12px;
    }
    .desktop-control .k {
      min-width: 0;
    }
    .control-key {
      display: block;
      margin-top: 4px;
    }
    .control-badges {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      margin-top: 5px;
    }
    .control-badge {
      padding: 1px 5px;
      border: 1px solid var(--border, #30343b);
      border-radius: 999px;
      color: var(--text-muted, #8d929b);
      font-size: 9px;
      line-height: 14px;
    }
    .control-badge.live {
      color: #65cdb0;
    }
    .control-badge.restart {
      color: #e3b35b;
    }
    .control-badge.saving {
      color: #8eb8ff;
    }
    .control-badge.host {
      color: #a8b3c7;
    }
    .control-actions {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 7px;
      width: min(310px, 52%);
    }
    .control-actions .control-input {
      width: min(210px, 100%);
    }
    .permission-request {
      white-space: nowrap;
    }
    .desktop-control.is-saving {
      opacity: 0.75;
    }
    .cfghint {
      margin: 8px 16px 0;
    }
  `,
})
export class DesktopControlsPanelView {
  private readonly store = inject(Store);
  private readonly connection = inject(ConnectionManagerService);

  readonly heading = input('Desktop controls');
  readonly description = input(
    'Wails, operating-system and security behaviour in one persisted panel.',
  );
  readonly precedence = input('');
  readonly help = input('');
  readonly excludedKeys = input<readonly string[]>([]);
  readonly showActions = input(true);
  readonly showRestartSummary = input(true);
  readonly permissions = input<readonly DesktopPermissionSnapshot[]>([]);
  readonly requestingPermission = input<DesktopPermissionID | null>(null);
  readonly permissionRequest = output<DesktopPermissionID>();

  readonly groups = this.store.selectSignal(selectDesktopControlGroups);
  readonly dirtyChanges = this.store.selectSignal(selectDirtyDesktopControlChanges);
  readonly hasDraftChanges = this.store.selectSignal(selectHasDirtyDesktopControls);
  readonly loading = this.store.selectSignal(desktopControlsFeature.selectLoading);
  readonly saving = this.store.selectSignal(desktopControlsFeature.selectSaving);
  readonly error = this.store.selectSignal(desktopControlsFeature.selectError);
  readonly restartSummary = this.store.selectSignal(desktopControlsFeature.selectRestartSummary);
  readonly dataState = computed<DesktopDataState>(() => {
    if (this.connection.offline()) return 'demo';
    if (this.loading()) return 'loading';
    if (this.error()) return 'unavailable';
    return 'live';
  });
  readonly visibleGroups = computed(() => {
    const excluded = new Set(this.excludedKeys());
    return this.groups()
      .map((group) => ({
        ...group,
        controls: group.controls.filter(({ key }) => !excluded.has(key)),
      }))
      .filter(({ controls }) => controls.length > 0);
  });

  setControl(control: DesktopControl, value: DesktopControlValue): void {
    if (this.saving() || control.value === value) return;
    this.store.dispatch(desktopControlsActions.editControl({ key: control.key, value }));
  }

  setChoice(control: DesktopControl, event: Event): void {
    this.setControl(control, (event.target as HTMLSelectElement).value);
  }

  setNumber(control: DesktopControl, event: Event): void {
    const value = (event.target as HTMLInputElement).valueAsNumber;
    if (!Number.isFinite(value)) return;
    this.setControl(control, value);
  }

  setText(control: DesktopControl, event: Event): void {
    this.setControl(control, (event.target as HTMLInputElement).value);
  }

  applyDraft(): void {
    const changes = this.dirtyChanges();
    if (this.saving() || changes.length === 0) return;
    this.store.dispatch(desktopControlsActions.applyDraft({ changes }));
  }

  discardDraft(): void {
    if (this.saving()) return;
    this.store.dispatch(desktopControlsActions.discardDraft());
  }

  resetDraft(): void {
    if (this.loading() || this.saving()) return;
    this.store.dispatch(desktopControlsActions.resetDraft());
  }

  retry(): void {
    if (this.loading() || this.saving()) return;
    this.store.dispatch(desktopControlsActions.load());
  }

  permissionForControl(key: string): DesktopPermissionSnapshot | null {
    const id = permissionIDForControl(key);
    return this.permissions().find((snapshot) => snapshot.id === id) ?? null;
  }

  canRequestPermission(key: string): boolean {
    const permission = this.permissionForControl(key);
    return (
      permission?.id === 'notifications' &&
      !['granted', 'restricted', 'unsupported'].includes(permission.host)
    );
  }

  requestPermission(key: string): void {
    if (this.requestingPermission() !== null || !this.canRequestPermission(key)) return;
    this.permissionRequest.emit('notifications');
  }
}

function permissionIDForControl(key: string): DesktopPermissionID | null {
  return PERMISSION_CONTROL_IDS[key] ?? null;
}
