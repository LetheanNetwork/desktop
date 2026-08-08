import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { SurfaceRoute } from '../surface-page';

/** A stored launch profile, as WListProfiles / WGetProfile return it. */
export interface OpencodeProfile {
  readonly name: string;
  readonly description?: string;
  readonly model?: string;
  readonly [key: string]: unknown;
}

/** Redacted provider row from WListImportedProviders — no raw keys. */
export interface OpencodeProviderView {
  readonly id: string;
  readonly source: string;
  readonly providerId: string;
  readonly name: string;
  readonly authType: string;
  readonly present: boolean;
  readonly masked: string;
}

const BRIDGE = 'dappco.re/lthn/desktop/pkg/opencode.WailsService';

const NEW_PROFILE_TEMPLATE = `{
  "name": "",
  "description": "",
  "model": "lthn/local"
}`;

@Component({
  selector: 'lthn-opencode-profiles-surface',
  imports: [],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="profiles">
      <header>
        <span class="icon"><i class="fa-solid fa-sliders" aria-hidden="true"></i></span>
        <div>
          <div class="eyebrow" i18n="OpenCode eyebrow profiles@@surface.opencode.profiles.eyebrow">
            OpenCode
          </div>
          <h2 i18n="OpenCode profiles title@@surface.opencode.profiles.title">Profiles</h2>
          <p i18n="OpenCode profiles subtitle@@surface.opencode.profiles.subtitle">
            Launch profiles narrow the model, tools, and providers a session may use.
          </p>
        </div>
        <button type="button" class="toggle" (click)="toggleEnabled()" [disabled]="busy()">
          @if (enabled()) {
            <i class="fa-solid fa-toggle-on" aria-hidden="true"></i>
            <ng-container i18n="OpenCode serve enabled@@surface.opencode.profiles.enabled"
              >Serve enabled</ng-container
            >
          } @else {
            <i class="fa-solid fa-toggle-off" aria-hidden="true"></i>
            <ng-container i18n="OpenCode serve disabled@@surface.opencode.profiles.disabled"
              >Serve disabled</ng-container
            >
          }
        </button>
      </header>

      @if (notice()) {
        <div class="notice" role="status">{{ notice() }}</div>
      }

      <div class="body">
        <aside>
          <div class="list-head">
            <span i18n="OpenCode profile list head@@surface.opencode.profiles.listHead">Stored</span>
            <button type="button" (click)="startNew()" [disabled]="busy()">
              <i class="fa-solid fa-plus" aria-hidden="true"></i>
              <ng-container i18n="OpenCode new profile@@surface.opencode.profiles.new"
                >New</ng-container
              >
            </button>
          </div>
          @for (p of profiles(); track p.name) {
            <button
              type="button"
              class="entry"
              [class.active]="p.name === selected()"
              (click)="select(p.name)"
            >
              <span class="name">{{ p.name }}</span>
              @if (p.model) {
                <span class="model">{{ p.model }}</span>
              }
            </button>
          } @empty {
            <div class="empty" i18n="OpenCode no profiles@@surface.opencode.profiles.empty">
              No stored profiles yet. The default profile always exists.
            </div>
          }

          <div class="list-head providers-head">
            <span i18n="OpenCode providers head@@surface.opencode.profiles.providersHead"
              >Imported providers</span
            >
            <button type="button" (click)="importFromHost()" [disabled]="busy()">
              <i class="fa-solid fa-file-import" aria-hidden="true"></i>
              <ng-container i18n="OpenCode import button@@surface.opencode.profiles.import"
                >Import</ng-container
              >
            </button>
          </div>
          @for (pr of providers(); track pr.id) {
            <div class="provider">
              <span class="name">{{ pr.name }}</span>
              @if (pr.present) {
                <span class="mono ok">{{ pr.masked }}</span>
              } @else {
                <span
                  class="mono muted"
                  i18n="OpenCode provider unconfigured@@surface.opencode.profiles.unconfigured"
                  >not configured</span
                >
              }
            </div>
          }
        </aside>

        <div class="editor">
          <label>
            <span i18n="OpenCode editor label@@surface.opencode.profiles.editorLabel"
              >Profile · JSON</span
            >
            <textarea
              [value]="editor()"
              (input)="onEditor($event)"
              spellcheck="false"
              rows="18"
            ></textarea>
          </label>
          <div class="editor-actions">
            <button type="button" class="primary" (click)="save()" [disabled]="busy()">
              <i class="fa-solid fa-floppy-disk" aria-hidden="true"></i>
              <ng-container i18n="OpenCode save profile@@surface.opencode.profiles.save"
                >Save profile</ng-container
              >
            </button>
            @if (selected()) {
              <button type="button" class="danger" (click)="remove()" [disabled]="busy()">
                <i class="fa-solid fa-trash" aria-hidden="true"></i>
                <ng-container i18n="OpenCode delete profile@@surface.opencode.profiles.delete"
                  >Delete</ng-container
                >
              </button>
            }
          </div>
        </div>
      </div>
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
    }
    .profiles {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      gap: 11px;
      padding: 20px;
      overflow: hidden auto;
      color: var(--fg-0);
      background: var(--ink-1);
      font-family: var(--font-sans);
    }
    header {
      display: flex;
      align-items: center;
      gap: 11px;
    }
    header > div {
      flex: 1;
    }
    .icon {
      display: grid;
      flex: 0 0 34px;
      width: 34px;
      height: 34px;
      place-items: center;
      color: var(--brand-300);
      background: rgba(64, 193, 197, 0.09);
      border: 1px solid rgba(64, 193, 197, 0.18);
      border-radius: 9px;
    }
    .eyebrow {
      color: var(--brand-300);
      font: 9px var(--font-mono);
      letter-spacing: 0.12em;
      text-transform: uppercase;
    }
    h2,
    p {
      margin: 0;
    }
    h2 {
      margin-top: 3px;
      font-size: 20px;
    }
    p {
      margin-top: 4px;
      color: var(--fg-2);
      font-size: 11px;
    }
    button {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 10px;
      color: var(--fg-0);
      background: var(--ink-2);
      border: 1px solid rgba(64, 193, 197, 0.2);
      border-radius: 6px;
      font: 11px var(--font-sans);
      cursor: pointer;
    }
    button:disabled {
      opacity: 0.5;
      cursor: default;
    }
    button.primary {
      color: var(--ink-1);
      background: var(--brand-300);
      border-color: var(--brand-300);
    }
    button.danger {
      color: #e2626b;
      border-color: rgba(226, 98, 107, 0.35);
    }
    .toggle {
      color: var(--brand-200);
      font: 10px var(--font-mono);
    }
    .notice {
      padding: 7px 9px;
      color: var(--brand-200);
      background: rgba(64, 193, 197, 0.06);
      border: 1px solid rgba(64, 193, 197, 0.17);
      border-radius: 6px;
      font: 9px var(--font-mono);
    }
    .body {
      display: grid;
      flex: 1;
      grid-template-columns: 280px 1fr;
      gap: 12px;
      min-height: 0;
    }
    aside {
      display: flex;
      flex-direction: column;
      gap: 6px;
      overflow-y: auto;
    }
    .list-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      color: var(--fg-2);
      font: 9px var(--font-mono);
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .providers-head {
      margin-top: 14px;
    }
    .entry {
      display: flex;
      flex-direction: column;
      align-items: start;
      gap: 2px;
      text-align: left;
    }
    .entry.active {
      border-color: rgba(64, 193, 197, 0.55);
      background: rgba(64, 193, 197, 0.08);
    }
    .entry .name {
      font-weight: 600;
    }
    .entry .model,
    .provider .mono {
      font: 9px var(--font-mono);
    }
    .entry .model {
      color: var(--fg-2);
    }
    .provider {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      padding: 6px 10px;
      border: 1px solid rgba(64, 193, 197, 0.12);
      border-radius: 6px;
      font-size: 11px;
    }
    .provider .ok {
      color: var(--brand-200);
    }
    .muted {
      color: var(--fg-2);
    }
    .empty {
      padding: 14px;
      color: var(--fg-2);
      border: 1px dashed rgba(64, 193, 197, 0.2);
      border-radius: 8px;
      font-size: 11px;
      text-align: center;
    }
    .editor {
      display: flex;
      flex-direction: column;
      gap: 8px;
      min-height: 0;
    }
    .editor label {
      display: flex;
      flex: 1;
      flex-direction: column;
      gap: 4px;
      color: var(--fg-2);
      font-size: 10px;
      min-height: 0;
    }
    .editor textarea {
      flex: 1;
      padding: 10px;
      color: var(--fg-0);
      background: var(--ink-2);
      border: 1px solid rgba(64, 193, 197, 0.2);
      border-radius: 8px;
      font: 11px var(--font-mono);
      resize: none;
    }
    .editor-actions {
      display: flex;
      gap: 8px;
    }
  `,
})
export class OpencodeProfilesSurface extends SurfaceRoute implements OnInit {
  private readonly bridge = inject(SurfaceBridgeService);

  readonly profiles = signal<readonly OpencodeProfile[]>([]);
  readonly providers = signal<readonly OpencodeProviderView[]>([]);
  readonly selected = signal('');
  readonly editor = signal(NEW_PROFILE_TEMPLATE);
  readonly enabled = signal(false);
  readonly notice = signal('');
  readonly busy = signal(false);

  ngOnInit(): void {
    void this.reload();
    void this.loadProviders();
    void this.loadEnabled();
  }

  onEditor(event: Event): void {
    this.editor.set((event.target as HTMLTextAreaElement).value);
  }

  select(name: string): void {
    this.selected.set(name);
    void this.loadSelected(name);
  }

  startNew(): void {
    this.selected.set('');
    this.editor.set(NEW_PROFILE_TEMPLATE);
  }

  async save(): Promise<void> {
    this.busy.set(true);
    try {
      const parsed = JSON.parse(this.editor()) as OpencodeProfile;
      if (!parsed || typeof parsed !== 'object' || !parsed.name) {
        throw new Error(
          $localize`:OpenCode profile name required@@surface.opencode.profiles.nameRequired:The profile needs a "name".`,
        );
      }
      await this.bridge.call(`${BRIDGE}.WSaveProfile`, [parsed]);
      this.selected.set(parsed.name);
      this.notice.set(
        $localize`:OpenCode profile saved@@surface.opencode.profiles.saved:Profile ${parsed.name}:name: saved.`,
      );
      await this.reload();
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  async remove(): Promise<void> {
    const name = this.selected();
    if (!name) return;
    this.busy.set(true);
    try {
      await this.bridge.call(`${BRIDGE}.WDeleteProfile`, [name]);
      this.notice.set(
        $localize`:OpenCode profile deleted@@surface.opencode.profiles.deleted:Profile ${name}:name: deleted.`,
      );
      this.startNew();
      await this.reload();
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  async toggleEnabled(): Promise<void> {
    this.busy.set(true);
    try {
      if (this.enabled()) {
        await this.bridge.call(`${BRIDGE}.WDisable`);
      } else {
        await this.bridge.call(`${BRIDGE}.WEnable`, [this.selected()]);
      }
      await this.loadEnabled();
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  async importFromHost(): Promise<void> {
    this.busy.set(true);
    try {
      await this.bridge.call(`${BRIDGE}.WImportFromHost`);
      this.notice.set(
        $localize`:OpenCode import done@@surface.opencode.profiles.imported:Host OpenCode configuration imported.`,
      );
      await this.loadProviders();
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  private async reload(): Promise<void> {
    try {
      const value = await this.bridge.call(`${BRIDGE}.WListProfiles`);
      this.profiles.set(Array.isArray(value) ? (value as OpencodeProfile[]) : []);
    } catch (error) {
      this.fail(error);
    }
  }

  private async loadSelected(name: string): Promise<void> {
    try {
      const value = await this.bridge.call(`${BRIDGE}.WGetProfile`, [name]);
      this.editor.set(JSON.stringify(value, null, 2));
    } catch (error) {
      this.fail(error);
    }
  }

  private async loadProviders(): Promise<void> {
    try {
      const value = await this.bridge.call(`${BRIDGE}.WListImportedProviders`);
      this.providers.set(Array.isArray(value) ? (value as OpencodeProviderView[]) : []);
    } catch {
      // Provider rows are informational; a bridge failure leaves the
      // list empty rather than blocking profile editing.
    }
  }

  private async loadEnabled(): Promise<void> {
    try {
      this.enabled.set((await this.bridge.call(`${BRIDGE}.WIsEnabled`)) === true);
    } catch {
      this.enabled.set(false);
    }
  }

  private fail(error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.notice.set(message);
  }
}
