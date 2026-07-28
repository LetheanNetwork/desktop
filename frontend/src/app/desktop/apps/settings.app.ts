import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  DestroyRef,
  Input,
  OnInit,
  PendingTasks,
  computed,
  inject,
  signal,
} from '@angular/core';
import { Store } from '@ngrx/store';
import { AppView } from './app-view';
import { AppNavItem } from '../desktop-route-tree';
import { Win } from '../desktop.data';
import { PreferencesService } from '../preferences.service';
import { WindowManagerService } from '../window-manager.service';
import {
  DesktopHostIntentService,
  type DesktopHostItem,
  type DesktopPermissionID,
  type DesktopPermissionSnapshot,
} from '../desktop-host-intent.service';
import { DesktopPermissionsBridgeService } from '../desktop-permissions-bridge.service';
import { desktopControlsActions } from '../../store/desktop-controls.actions';
import { DesktopControl, DesktopControlValue } from '../../store/desktop-controls.models';
import {
  desktopControlsFeature,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectDraftDesktopControls,
  selectHasDirtyDesktopControls,
} from '../../store/desktop-controls.reducer';

const CURATED_PREFERENCE_KEYS = new Set([
  'desktop.theme.design',
  'desktop.theme.custom_hue',
  'desktop.theme.custom_name',
  'desktop.theme.wallpaper',
  'desktop.shell.taskbar_edge',
  'desktop.shell.show_icons',
  'desktop.shell.show_widgets',
]);

@Component({
  selector: 'lthn-settings-app',
  standalone: true,
  imports: [],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="rail">
      @for (item of nav; track item[0]; let last = $last) {
        <a
          [class.on]="(win.sub || 'interface') === item[0]"
          [class.last]="last"
          (click)="wm.setSub(win.id, item[0])"
        >
          <lthn-icon [attr.name]="item[1]" [attr.aria-label]="item[2]" size="15"></lthn-icon>
        </a>
      }
    </nav>
    <div class="appbody">
      @switch (win.sub || 'interface') {
        @case ('translations') {
          <div class="ctoolbar">
            <h1 i18n="Translations settings heading@@settings.translations.heading">
              Translations
            </h1>
            <span
              class="cfgsrc"
              i18n="Translation file breadcrumb@@settings.translations.fileBreadcrumb"
              >locales/{{ prefs.lang() }}.yaml · corego/pkg/i18n</span
            >
          </div>
          <div class="i18n">
            <div class="i18nrow i18nhead">
              <span i18n="Translation key column@@settings.translations.column.key">Key</span
              ><span i18n="English source column@@settings.translations.column.english"
                >English</span
              ><span>{{ prefs.lang().toUpperCase() }}</span>
            </div>
            @for (row of i18nKeys; track row[0]) {
              <div class="i18nrow">
                <span class="i18nk">{{ row[0] }}</span>
                <span class="i18nen">{{ row[1] }}</span>
                <span class="i18ncell" contenteditable="true">{{ translated(row) }}</span>
              </div>
            }
          </div>
          <p class="cfghint" i18n="Translation editor help@@settings.translations.help">
            <lthn-icon name="circle-info" size="11"></lthn-icon> Click a
            {{ prefs.lang().toUpperCase() }} cell to edit — writes to
            <code>locales/{{ prefs.lang() }}.yaml</code> via corego/pkg/i18n.
          </p>
        }
        @default {
          <div class="ctoolbar">
            <h1 i18n="UI preferences heading@@settings.preferences.heading">UI preferences</h1>
          </div>
          @if (importItem(); as item) {
            <p class="controls-import-intent" role="status">
              <strong>{{ item.name }}</strong>
              <span i18n="Settings native import hand-off@@settings.import.ready">
                is ready for import review. No changes have been applied.
              </span>
            </p>
          }
          <div class="settings-actions" aria-label="Settings draft actions">
            <button
              type="button"
              data-action="reset-settings"
              [disabled]="controlsLoading() || controlsSaving()"
              (click)="resetDraft()"
            >
              Reset
            </button>
            <button
              type="button"
              data-action="discard-settings"
              [disabled]="!hasDraftChanges() || controlsSaving()"
              (click)="discardDraft()"
            >
              Discard
            </button>
            <button
              type="button"
              class="primary"
              data-action="apply-settings"
              [disabled]="!hasDraftChanges() || controlsSaving()"
              (click)="applyDraft()"
            >
              {{ controlsSaving() ? 'Applying…' : 'Apply' }}
            </button>
          </div>
          @if (restartSummary()) {
            <p class="controls-restart-summary" role="status">{{ restartSummary() }}</p>
          }
          <div class="setgroup">
            <span class="glab" i18n="Settings group@@settings.group.appearance">Appearance</span>
            <div class="setrow">
              <div class="k" i18n="Design preference@@settings.design">
                Design<small>Brand token ramp</small>
              </div>
              <div class="prefseg" role="group">
                <button
                  [class.on]="preferenceValue('desktop.theme.design', 'lethean') === 'lethean'"
                  (click)="editPreference('desktop.theme.design', 'lethean')"
                >
                  <ng-container i18n="Lethean design option@@settings.design.lethean"
                    >Lethean</ng-container
                  >
                </button>
                <button
                  [class.on]="preferenceValue('desktop.theme.design', 'lethean') === 'custom'"
                  (click)="editPreference('desktop.theme.design', 'custom')"
                  i18n="Custom design option@@settings.design.custom"
                >
                  Custom
                </button>
              </div>
            </div>
            @if (preferenceValue('desktop.theme.design', 'lethean') === 'custom') {
              <div class="themed">
                <input
                  class="cfgin"
                  style="width:100%"
                  [value]="preferenceValue('desktop.theme.custom_name', 'Host UK')"
                  (input)="setCustomName($event)"
                  aria-label="Design name"
                  i18n-aria-label="Custom design name input@@settings.design.nameLabel"
                />
                <div class="thlab" i18n="Accent colour label@@settings.design.accent">Accent</div>
                <div class="thswatches">
                  @for (hue of hues; track hue[0]) {
                    <button
                      class="thsw"
                      [class.on]="preferenceValue('desktop.theme.custom_hue', 305) === hue[0]"
                      [title]="hue[1]"
                      [style.background]="'oklch(0.62 0.15 ' + hue[0] + ')'"
                      (click)="editPreference('desktop.theme.custom_hue', hue[0])"
                    ></button>
                  }
                </div>
                <div
                  class="thlab"
                  i18n="Generated colour ramp label@@settings.design.generatedRamp"
                >
                  Generated ramp
                </div>
                <div class="thramp">
                  @for (shade of [200, 300, 400, 500, 600, 700]; track shade) {
                    <span [style.background]="'var(--brand-' + shade + ')'"></span>
                  }
                </div>
              </div>
            }
            <div class="setrow">
              <div class="k" i18n="Wallpaper preference@@settings.wallpaper">
                Wallpaper<small>Desktop background</small>
              </div>
              <div class="prefseg" role="group">
                @for (wallpaper of wallpapers; track wallpaper) {
                  <button
                    [class.on]="preferenceValue('desktop.theme.wallpaper', 'aurora') === wallpaper"
                    (click)="editPreference('desktop.theme.wallpaper', wallpaper)"
                  >
                    {{ wallpaperLabel(wallpaper) }}
                  </button>
                }
              </div>
            </div>
          </div>
          <div class="setgroup">
            <span class="glab" i18n="Settings group@@settings.group.desktop">Desktop</span>
            <div class="setrow">
              <div class="k" i18n="Taskbar edge preference@@settings.taskbarEdge">
                Taskbar edge<small>Dock the taskbar to any screen edge</small>
              </div>
              <div class="prefseg" role="group">
                @for (edge of edges; track edge) {
                  <button
                    [class.on]="preferenceValue('desktop.shell.taskbar_edge', 'bottom') === edge"
                    (click)="editPreference('desktop.shell.taskbar_edge', edge)"
                  >
                    {{ edgeLabel(edge) }}
                  </button>
                }
              </div>
            </div>
            <div class="setrow">
              <div class="k" i18n="Desktop layout preference@@settings.layout">
                Layout<small>Floating windows, app shell, or device frame</small>
              </div>
              <div class="prefseg" role="group">
                <button
                  [class.on]="wm.view() === 'desktop'"
                  (click)="wm.setView('desktop')"
                  i18n="Desktop layout option@@settings.layout.desktop"
                >
                  Desktop
                </button>
                <button
                  [class.on]="wm.view() === 'shell'"
                  (click)="wm.setView('shell')"
                  i18n="App shell layout option@@settings.layout.appShell"
                >
                  App shell
                </button>
                <button
                  [class.on]="wm.view() === 'device'"
                  (click)="wm.setView('device')"
                  i18n="Device layout option@@settings.layout.device"
                >
                  Device
                </button>
              </div>
            </div>
            <div class="setrow">
              <div class="k" i18n="Device size preference@@settings.deviceSize">
                Device size<small>Small, large, or full</small>
              </div>
              <div class="prefseg" role="group">
                @for (device of devices; track device) {
                  <button [class.on]="wm.device() === device" (click)="wm.setDevice(device)">
                    {{ deviceLabel(device) }}
                  </button>
                }
              </div>
            </div>
            <div class="setrow">
              <div class="k" i18n="Desktop icon preference@@settings.desktopIcons">
                Desktop icons<small>Show launcher tiles on the wallpaper</small>
              </div>
              <div class="prefseg" role="group">
                <button
                  [class.on]="preferenceValue('desktop.shell.show_icons', true) === true"
                  (click)="editPreference('desktop.shell.show_icons', true)"
                  i18n="Enabled option@@common.on"
                >
                  On
                </button>
                <button
                  [class.on]="preferenceValue('desktop.shell.show_icons', true) === false"
                  (click)="editPreference('desktop.shell.show_icons', false)"
                  i18n="Disabled option@@common.off"
                >
                  Off
                </button>
              </div>
            </div>
            <div class="setrow">
              <div class="k" i18n="Desktop widgets preference@@settings.widgets">
                Widgets<small>Chart, world clock &amp; package status</small>
              </div>
              <div class="prefseg" role="group">
                <button
                  [class.on]="preferenceValue('desktop.shell.show_widgets', true) === true"
                  (click)="editPreference('desktop.shell.show_widgets', true)"
                  i18n="Enabled option@@common.on"
                >
                  On
                </button>
                <button
                  [class.on]="preferenceValue('desktop.shell.show_widgets', true) === false"
                  (click)="editPreference('desktop.shell.show_widgets', false)"
                  i18n="Disabled option@@common.off"
                >
                  Off
                </button>
              </div>
            </div>
          </div>
          <section class="desktop-controls-panel" aria-labelledby="desktop-controls-heading">
            <div class="controls-heading">
              <div>
                <h1 id="desktop-controls-heading">Desktop controls</h1>
                <p>Wails, operating-system and security behaviour in one persisted panel.</p>
              </div>
            </div>

            @if (controlsError()) {
              <p class="controls-error" role="alert">{{ controlsError() }}</p>
            }
            @if (controlsLoading() && controlGroups().length === 0) {
              <p class="controls-status">Loading desktop controls…</p>
            }

            @for (group of visibleControlGroups(); track group.name) {
              <div class="setgroup control-group">
                <span class="glab">{{ group.name }}</span>
                @for (control of group.controls; track control.key) {
                  <div
                    class="setrow desktop-control"
                    [attr.data-control-key]="control.key"
                    [class.is-saving]="controlsSaving()"
                  >
                    <div class="k">
                      {{ control.label }}
                      <small>{{ control.description }}</small>
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
                        @if (controlsSaving()) {
                          <span class="control-badge saving">Saving…</span>
                        }
                        @if (permissionForControl(control.key); as permission) {
                          <span class="control-badge host">
                            Host: {{ permission.host }}
                          </span>
                        }
                      </span>
                    </div>

                    <div class="control-actions">
                      @switch (control.kind) {
                        @case ('toggle') {
                          <div class="prefseg" role="group" [attr.aria-label]="control.label">
                            <button
                              [class.on]="control.value === true"
                              [disabled]="controlsSaving()"
                              (click)="setToggle(control, true)"
                            >
                              On
                            </button>
                            <button
                              [class.on]="control.value === false"
                              [disabled]="controlsSaving()"
                              (click)="setToggle(control, false)"
                            >
                              Off
                            </button>
                          </div>
                        }
                        @case ('select') {
                          <select
                            class="cfgin control-input"
                            [value]="control.value"
                            [disabled]="controlsSaving()"
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
                            [disabled]="controlsSaving()"
                            [attr.aria-label]="control.label"
                            (change)="setNumber(control, $event)"
                          />
                        }
                        @default {
                          <input
                            class="cfgin control-input"
                            type="text"
                            [value]="control.value"
                            [disabled]="controlsSaving()"
                            [attr.aria-label]="control.label"
                            (change)="setText(control, $event)"
                          />
                        }
                      }
                      @if (canRequestPermission(control.key)) {
                        <button
                          type="button"
                          class="permission-request"
                          [attr.data-action]="'request-permission-notifications'"
                          [disabled]="requestingPermission() !== null"
                          (click)="requestPermission('notifications')"
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
            @if (permissionError()) {
              <p class="controls-error" role="alert">{{ permissionError() }}</p>
            }
          </section>
        }
      }
    </div>
  `,
  styles: `
    .desktop-controls-panel {
      margin-top: 22px;
      padding-bottom: 24px;
    }
    .settings-actions {
      display: flex;
      justify-content: flex-end;
      gap: 7px;
      margin: 0 16px 12px;
    }
    .settings-actions button {
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
    .settings-actions button:disabled {
      cursor: default;
      opacity: 0.48;
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
      margin: 4px 0 0;
      color: var(--text-muted, #8d929b);
      font-size: 11px;
    }
    .controls-error {
      margin: 0 16px 12px;
      padding: 8px 10px;
      border: 1px solid color-mix(in srgb, #ef6b73 45%, transparent);
      border-radius: 7px;
      background: color-mix(in srgb, #ef6b73 10%, transparent);
      color: #ef9aa0;
      font-size: 11px;
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
    .control-input {
      width: min(210px, 42%);
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
      padding: 5px 8px;
      border: 1px solid var(--border, #30343b);
      border-radius: 6px;
      background: var(--surface-2, #181a20);
      color: var(--text, #e7e9ed);
      white-space: nowrap;
    }
    .desktop-control.is-saving {
      opacity: 0.75;
    }
  `,
})
export class SettingsApp implements AppView, OnInit {
  @Input({ required: true }) win!: Win;
  @Input() nav: AppNavItem[] = [];

  readonly prefs = inject(PreferencesService);
  readonly wm = inject(WindowManagerService);
  private readonly hostIntents = inject(DesktopHostIntentService);
  private readonly permissions = inject(DesktopPermissionsBridgeService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly pendingTasks = inject(PendingTasks);
  private readonly store = inject(Store);
  readonly importItem = signal<DesktopHostItem | null>(null);
  readonly permissionSnapshots = signal<readonly DesktopPermissionSnapshot[]>([]);
  readonly permissionError = signal('');
  readonly requestingPermission = signal<DesktopPermissionID | null>(null);
  readonly draftControls = this.store.selectSignal(selectDraftDesktopControls);
  readonly controlGroups = this.store.selectSignal(selectDesktopControlGroups);
  readonly visibleControlGroups = computed(() =>
    this.controlGroups()
      .map((group) => ({
        ...group,
        controls: group.controls.filter(({ key }) => !CURATED_PREFERENCE_KEYS.has(key)),
      }))
      .filter(({ controls }) => controls.length > 0),
  );
  readonly controlsLoading = this.store.selectSignal(desktopControlsFeature.selectLoading);
  readonly controlsError = this.store.selectSignal(desktopControlsFeature.selectError);
  readonly controlsSaving = this.store.selectSignal(desktopControlsFeature.selectSaving);
  readonly restartSummary = this.store.selectSignal(desktopControlsFeature.selectRestartSummary);
  readonly dirtyChanges = this.store.selectSignal(selectDirtyDesktopControlChanges);
  readonly hasDraftChanges = this.store.selectSignal(selectHasDirtyDesktopControls);
  readonly wallpapers = ['aurora', 'dusk', 'mist', 'graphite'] as const;
  readonly devices = ['small', 'large', 'full'] as const;
  readonly edges = ['top', 'right', 'bottom', 'left'] as const;
  readonly hues: [number, string][] = [
    [190, $localize`:Accent colour@@settings.accent.teal:Teal`],
    [305, $localize`:Accent colour@@settings.accent.viPurple:Vi purple`],
    [250, $localize`:Accent colour@@settings.accent.blue:Blue`],
    [150, $localize`:Accent colour@@settings.accent.green:Green`],
    [40, $localize`:Accent colour@@settings.accent.ember:Ember`],
    [340, $localize`:Accent colour@@settings.accent.magenta:Magenta`],
  ];
  readonly i18nKeys: [string, string][] = [
    ['menu.file', 'File'],
    ['menu.edit', 'Edit'],
    ['menu.view', 'View'],
    ['action.save', 'Save'],
    ['action.cancel', 'Cancel'],
    ['action.open', 'Open'],
    ['session.lock', 'Lock screen'],
    ['session.logout', 'Log out'],
    ['app.control', 'Control'],
    ['app.games', 'Games'],
    ['app.notepad', 'Notepad'],
  ];
  readonly translations: Record<string, Record<string, string>> = {
    fr: {
      'menu.file': 'Fichier',
      'menu.edit': 'Édition',
      'menu.view': 'Affichage',
      'action.save': 'Enregistrer',
    },
    de: {
      'menu.file': 'Datei',
      'menu.edit': 'Bearbeiten',
      'menu.view': 'Ansicht',
      'action.save': 'Speichern',
    },
    cy: {
      'menu.file': 'Ffeil',
      'menu.edit': 'Golygu',
      'menu.view': 'Golwg',
      'action.save': 'Cadw',
    },
    es: {
      'menu.file': 'Archivo',
      'menu.edit': 'Editar',
      'menu.view': 'Ver',
      'action.save': 'Guardar',
    },
    ja: { 'menu.file': 'ファイル', 'menu.edit': '編集', 'menu.view': '表示' },
  };

  ngOnInit(): void {
    const offItems = this.hostIntents.onItems('settings', (items) => {
      this.importItem.set(items[0] ?? null);
    });
    this.destroyRef.onDestroy(offItems);
    this.store.dispatch(desktopControlsActions.load());
    this.pendingTasks.run(async () => {
      try {
        this.permissionSnapshots.set(await this.permissions.status());
      } catch (error) {
        this.permissionError.set(errorMessage(error));
      }
    });
  }

  permissionForControl(key: string): DesktopPermissionSnapshot | null {
    const id = permissionIDForControl(key);
    return this.permissionSnapshots().find((snapshot) => snapshot.id === id) ?? null;
  }

  canRequestPermission(key: string): boolean {
    const permission = this.permissionForControl(key);
    return (
      permission?.id === 'notifications' &&
      !['granted', 'restricted', 'unsupported'].includes(permission.host)
    );
  }

  requestPermission(id: DesktopPermissionID): void {
    if (this.requestingPermission() !== null || id !== 'notifications') return;
    this.requestingPermission.set(id);
    this.permissionError.set('');
    this.pendingTasks.run(async () => {
      try {
        const snapshot = await this.permissions.request(id);
        this.permissionSnapshots.update((current) =>
          current.map((item) => (item.id === snapshot.id ? snapshot : item)),
        );
      } catch (error) {
        this.permissionError.set(errorMessage(error));
      } finally {
        this.requestingPermission.set(null);
      }
    });
  }

  setToggle(control: DesktopControl, value: boolean): void {
    this.setControl(control, value);
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

  private setControl(control: DesktopControl, value: boolean | number | string): void {
    if (this.controlsSaving() || control.value === value) return;
    this.store.dispatch(desktopControlsActions.editControl({ key: control.key, value }));
  }

  setCustomName(event: Event): void {
    this.editPreference('desktop.theme.custom_name', (event.target as HTMLInputElement).value);
  }

  preferenceValue(key: string, fallback: DesktopControlValue): DesktopControlValue {
    const value = this.draftControls().find((control) => control.key === key)?.value;
    return value === undefined ? fallback : value;
  }

  editPreference(key: string, value: DesktopControlValue): void {
    if (this.controlsSaving()) return;
    this.store.dispatch(desktopControlsActions.editControl({ key, value }));
  }

  applyDraft(): void {
    const changes = this.dirtyChanges();
    if (this.controlsSaving() || changes.length === 0) return;
    this.store.dispatch(desktopControlsActions.applyDraft({ changes }));
  }

  discardDraft(): void {
    if (this.controlsSaving()) return;
    this.store.dispatch(desktopControlsActions.discardDraft());
  }

  resetDraft(): void {
    if (this.controlsSaving() || this.controlsLoading()) return;
    this.store.dispatch(desktopControlsActions.resetDraft());
  }

  wallpaperLabel(wallpaper: string): string {
    const labels: Record<string, string> = {
      aurora: $localize`:Wallpaper name@@settings.wallpaper.aurora:aurora`,
      dusk: $localize`:Wallpaper name@@settings.wallpaper.dusk:dusk`,
      mist: $localize`:Wallpaper name@@settings.wallpaper.mist:mist`,
      graphite: $localize`:Wallpaper name@@settings.wallpaper.graphite:graphite`,
    };
    return labels[wallpaper] ?? wallpaper;
  }

  edgeLabel(edge: string): string {
    const labels: Record<string, string> = {
      top: $localize`:Taskbar edge@@settings.edge.top:top`,
      right: $localize`:Taskbar edge@@settings.edge.right:right`,
      bottom: $localize`:Taskbar edge@@settings.edge.bottom:bottom`,
      left: $localize`:Taskbar edge@@settings.edge.left:left`,
    };
    return labels[edge] ?? edge;
  }

  deviceLabel(device: string): string {
    const labels: Record<string, string> = {
      small: $localize`:Device size@@settings.device.small:small`,
      large: $localize`:Device size@@settings.device.large:large`,
      full: $localize`:Device size@@settings.device.full:full`,
    };
    return labels[device] ?? device;
  }

  translated(row: [string, string]): string {
    const lang = this.prefs.lang();
    return lang === 'en' ? row[1] : (this.translations[lang]?.[row[0]] ?? '');
  }
}

const PERMISSION_CONTROL_IDS: Readonly<Record<string, DesktopPermissionID>> = {
  'desktop.permissions.microphone': 'microphone',
  'desktop.permissions.camera': 'camera',
  'desktop.permissions.geolocation': 'geolocation',
  'desktop.permissions.notifications': 'notifications',
  'desktop.permissions.clipboard_read': 'clipboard-read',
};

function permissionIDForControl(key: string): DesktopPermissionID | null {
  return PERMISSION_CONTROL_IDS[key] ?? null;
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message
    ? error.message
    : 'The native permission state is unavailable.';
}
