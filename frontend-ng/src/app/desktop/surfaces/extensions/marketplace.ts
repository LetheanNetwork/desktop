import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { SurfaceRoute } from '../surface-page';

export interface PluginViewManifest {
  readonly ID?: string;
  readonly Label?: string;
  readonly Icon?: string;
  readonly Kind?: string;
  readonly Source?: string;
  readonly Capabilities?: readonly string[];
}

export interface ManifestWithViews {
  readonly Plugin?: {
    readonly Code?: string;
    readonly Views?: readonly PluginViewManifest[];
  } | null;
}

export interface ViewDiscriminator {
  readonly iconClass: string;
  readonly label: string;
  readonly bodyCopy: string;
}

export const VIEW_KIND_IFRAME = 'iframe';
export const VIEW_KIND_LIT = 'lit';
export const CAPABILITY_SESSION_TOKEN = 'session-token';
export const SESSION_TOKEN_PROMPT_COPY =
  '%d can read and write any data you have unlocked in lthn, for up to 30 minutes per unlock. Allow?';

export function discriminateView(view: PluginViewManifest): ViewDiscriminator {
  if (view.Kind === VIEW_KIND_LIT) {
    return {
      iconClass: 'fa-cubes',
      label: 'Deeply integrated',
      bodyCopy: 'Full app access — runs alongside the rest of the shell.',
    };
  }
  if ((view.Capabilities ?? []).includes(CAPABILITY_SESSION_TOKEN)) {
    return {
      iconClass: 'fa-shield-halved',
      label: 'Isolated webapp — requests account access',
      bodyCopy:
        'Runs in its own isolated webview. Asks to read and write any data you have unlocked, for up to 30 minutes per unlock.',
    };
  }
  return {
    iconClass: 'fa-shield',
    label: 'Isolated webapp',
    bodyCopy: 'Runs in its own isolated webview. Does not request access to your account data.',
  };
}

export function hasAnyViews(manifest: ManifestWithViews | null | undefined): boolean {
  return !!manifest?.Plugin?.Views?.length;
}

export function viewSummaryLine(manifest: ManifestWithViews | null | undefined): string {
  if (!hasAnyViews(manifest)) return '';
  const labels = (manifest?.Plugin?.Views ?? [])
    .map((view) => (view.Label ?? view.ID ?? '').trim())
    .filter(Boolean);
  return labels.length ? `Provides views: ${labels.join(', ')}` : '';
}

export function wantsSessionTokenCapability(
  manifest: ManifestWithViews | null | undefined,
): boolean {
  return !!manifest?.Plugin?.Views?.some((view) =>
    (view.Capabilities ?? []).includes(CAPABILITY_SESSION_TOKEN),
  );
}

export function sessionTokenPrompt(displayName: string): string {
  return SESSION_TOKEN_PROMPT_COPY.replace('%d', displayName.trim() || 'This plugin');
}

interface MarketplaceEntry {
  readonly code: string;
  readonly name: string;
  readonly version: string;
  readonly summary: string;
  readonly installed: boolean;
  readonly manifest: ManifestWithViews;
}

const CATALOGUE: readonly MarketplaceEntry[] = [
  {
    code: 'opencode',
    name: 'OpenCode',
    version: '1.4.2',
    summary: 'Local coding agent with project workspaces and terminal sessions.',
    installed: true,
    manifest: {
      Plugin: {
        Code: 'opencode',
        Views: [
          {
            ID: 'opencode',
            Label: 'OpenCode',
            Icon: 'fa-terminal',
            Kind: 'iframe',
            Source: '/v1/api/plugin/opencode/',
            Capabilities: ['session-token'],
          },
        ],
      },
    },
  },
  {
    code: 'grafana-local',
    name: 'Grafana Local',
    version: '0.8.1',
    summary: 'A local, isolated telemetry view for your own observability data.',
    installed: false,
    manifest: {
      Plugin: {
        Code: 'grafana-local',
        Views: [
          {
            ID: 'grafana-local:dashboard',
            Label: 'Grafana',
            Icon: 'fa-chart-line',
            Kind: 'iframe',
            Source: 'http://127.0.0.1:4319/',
          },
        ],
      },
    },
  },
  {
    code: 'lthn-lab-tools',
    name: 'Lethean Lab Tools',
    version: '2.1.0',
    summary: 'First-party training diagnostics integrated with the desktop shell.',
    installed: true,
    manifest: {
      Plugin: {
        Code: 'lthn-lab-tools',
        Views: [
          {
            ID: 'lthn-lab-tools:diagnostics',
            Label: 'Lab diagnostics',
            Icon: 'fa-microscope',
            Kind: 'lit',
            Source: 'lthn-lab-diagnostics',
          },
        ],
      },
    },
  },
];

@Component({
  selector: 'lthn-extensions-marketplace-surface',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="marketplace">
      <header>
        <div class="title">
          <span class="icon"><i class="fa-solid fa-store" aria-hidden="true"></i></span>
          <div>
            <div class="eyebrow" i18n="Extensions eyebrow@@surface.extensions.eyebrow">
              Extensions
            </div>
            <h2 i18n="Marketplace title@@surface.extensions.marketplace.title">Marketplace</h2>
            <p i18n="Marketplace subtitle@@surface.extensions.marketplace.subtitle">
              Inspect what a plugin adds and what it can access before installation.
            </p>
          </div>
        </div>
        <label>
          <i class="fa-solid fa-magnifying-glass" aria-hidden="true"></i>
          <span class="sr-only" i18n="Marketplace search@@surface.extensions.marketplace.search"
            >Search marketplace</span
          >
          <input
            type="search"
            placeholder="Search extensions"
            i18n-placeholder="
              Marketplace search placeholder@@surface.extensions.marketplace.searchPlaceholder"
            [value]="query()"
            (input)="setQuery($event)"
          />
        </label>
      </header>

      <div class="catalogue">
        @for (entry of visible(); track entry.code) {
          <article [attr.data-installed]="entry.installed">
            <div class="entry-head">
              <span class="mark">{{ entry.name.slice(0, 1) }}</span>
              <div>
                <h3>{{ entry.name }}</h3>
                <span>{{ entry.code }} · v{{ entry.version }}</span>
              </div>
              <span class="state">{{ entry.installed ? 'installed' : 'available' }}</span>
            </div>
            <p>{{ entry.summary }}</p>
            <div class="summary">{{ viewSummaryLine(entry.manifest) }}</div>
            @for (view of entry.manifest.Plugin?.Views ?? []; track view.ID) {
              @let discriminator = discriminateView(view);
              <div class="discriminator">
                <i class="fa-solid {{ discriminator.iconClass }}" aria-hidden="true"></i>
                <div>
                  <strong>{{ view.Label || view.ID }} · {{ discriminator.label }}</strong>
                  <span>{{ discriminator.bodyCopy }}</span>
                </div>
              </div>
            }
            @if (wantsSessionTokenCapability(entry.manifest)) {
              <div class="prompt">
                <i class="fa-solid fa-shield-halved" aria-hidden="true"></i>
                <span>{{ sessionTokenPrompt(entry.name) }}</span>
              </div>
            }
            <button type="button" (click)="select(entry)">
              <i
                class="fa-solid"
                [class.fa-gear]="entry.installed"
                [class.fa-download]="!entry.installed"
                aria-hidden="true"
              ></i>
              {{ entry.installed ? 'Configure' : 'Review installation' }}
            </button>
          </article>
        } @empty {
          <div class="empty" i18n="Marketplace empty@@surface.extensions.marketplace.empty">
            No extensions match this search.
          </div>
        }
      </div>
      @if (selected()) {
        <footer role="status">
          {{ selected()!.name }} ·
          <span i18n="Marketplace local review status@@surface.extensions.marketplace.localReview"
            >review opened locally; no installation has been performed.</span
          >
        </footer>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
    }
    .marketplace {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      overflow: auto;
      color: var(--fg-0);
      background:
        radial-gradient(circle at 70% 0, rgba(64, 193, 197, 0.08), transparent 35%), var(--ink-1);
      font-family: var(--font-sans);
    }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 20px;
      padding: 22px 24px 18px;
      border-bottom: 1px solid var(--line-1);
    }
    .title,
    .entry-head,
    .discriminator,
    .prompt {
      display: flex;
      align-items: flex-start;
      gap: 11px;
    }
    .icon,
    .mark {
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
    h3,
    p {
      margin: 0;
    }
    h2 {
      margin-top: 3px;
      font-size: 21px;
      letter-spacing: -0.025em;
    }
    header p,
    article > p {
      margin-top: 5px;
      color: var(--fg-2);
      font-size: 11px;
      line-height: 1.5;
    }
    header label {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 230px;
      padding: 8px 10px;
      color: var(--fg-3);
      background: var(--ink-2);
      border: 1px solid var(--line-1);
      border-radius: 7px;
    }
    input {
      width: 100%;
      color: var(--fg-0);
      background: transparent;
      border: 0;
      outline: 0;
      font: 11px var(--font-sans);
    }
    .catalogue {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(270px, 1fr));
      gap: 10px;
      padding: 18px 24px;
    }
    article {
      padding: 15px;
      background: rgba(255, 255, 255, 0.018);
      border: 1px solid var(--line-1);
      border-radius: 10px;
    }
    article[data-installed='true'] {
      border-color: rgba(64, 193, 197, 0.2);
    }
    .entry-head {
      align-items: center;
    }
    .entry-head > div {
      flex: 1;
      min-width: 0;
    }
    h3 {
      font-size: 13px;
      font-weight: 560;
    }
    .entry-head span,
    .summary {
      color: var(--fg-3);
      font: 9px var(--font-mono);
    }
    .state {
      padding: 3px 6px;
      border: 1px solid var(--line-1);
      border-radius: 999px;
    }
    .summary {
      margin: 13px 0 7px;
    }
    .discriminator,
    .prompt {
      margin-top: 7px;
      padding: 9px 10px;
      background: rgba(0, 0, 0, 0.14);
      border: 1px solid var(--line-1);
      border-radius: 7px;
    }
    .discriminator i,
    .prompt i {
      margin-top: 2px;
      color: var(--brand-300);
      font-size: 11px;
    }
    .discriminator div {
      display: flex;
      flex-direction: column;
      gap: 3px;
    }
    .discriminator strong {
      color: var(--fg-1);
      font-size: 10px;
      font-weight: 500;
    }
    .discriminator span,
    .prompt span {
      color: var(--fg-2);
      font-size: 9px;
      line-height: 1.5;
    }
    .prompt {
      background: rgba(64, 193, 197, 0.06);
      border-color: rgba(64, 193, 197, 0.17);
    }
    button {
      margin-top: 12px;
      padding: 7px 9px;
      color: var(--brand-200);
      background: rgba(64, 193, 197, 0.08);
      border: 1px solid rgba(64, 193, 197, 0.22);
      border-radius: 7px;
      font: 9px var(--font-mono);
      cursor: pointer;
    }
    button i {
      margin-right: 5px;
    }
    footer {
      margin: 0 24px 16px;
      padding: 9px 11px;
      color: var(--brand-200);
      background: rgba(64, 193, 197, 0.06);
      border: 1px solid rgba(64, 193, 197, 0.16);
      border-radius: 7px;
      font-size: 10px;
    }
    .empty {
      grid-column: 1/-1;
      padding: 50px;
      color: var(--fg-3);
      text-align: center;
      font-size: 11px;
    }
    .sr-only {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip: rect(0, 0, 0, 0);
      white-space: nowrap;
    }
    @media (max-width: 680px) {
      header {
        align-items: stretch;
        flex-direction: column;
      }
      header label {
        min-width: 0;
      }
    }
  `,
})
export class ExtensionsMarketplaceSurface extends SurfaceRoute {
  readonly discriminateView = discriminateView;
  readonly sessionTokenPrompt = sessionTokenPrompt;
  readonly viewSummaryLine = viewSummaryLine;
  readonly wantsSessionTokenCapability = wantsSessionTokenCapability;
  readonly query = signal('');
  readonly selected = signal<MarketplaceEntry | null>(null);

  visible(): readonly MarketplaceEntry[] {
    const query = this.query().trim().toLocaleLowerCase('en-GB');
    return query
      ? CATALOGUE.filter((entry) =>
          [entry.name, entry.code, entry.summary]
            .join(' ')
            .toLocaleLowerCase('en-GB')
            .includes(query),
        )
      : CATALOGUE;
  }

  setQuery(event: Event): void {
    this.query.set((event.target as HTMLInputElement).value);
  }

  select(entry: MarketplaceEntry): void {
    this.selected.set(entry);
  }
}
