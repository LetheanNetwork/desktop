// SPDX-Licence-Identifier: EUPL-1.2
// Office · Files — <lthn-view-files>
//
// Local indexed filesystem browser. Two-pane layout: locations (left, with
// disk-free meter at the bottom) · recent files list (right). Calm,
// observational tone — the Monday-morning surface.
//
// Two-shell: standalone (with chrome) when spawned directly, embedded (no
// chrome) when mounted inside <lthn-app-shell>. Mirrors the welcome /
// settings window pattern in ../../ops/welcome-window.ts.
//
// Data: fixture arrays only for v1. Real source is a future Go service
// at `pkg/files` — a local indexed filesystem walker (mirror of macOS
// Spotlight / Recoll) that writes its index into go-store. The lthn /
// models folder is brand-coloured because it's a Lethean-managed
// location (~/Lethean/conf/models/); other locations are plain. When
// bindings land, swap FIXTURE_LOCATIONS / FIXTURE_RECENT for fetches.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import { clampRows } from "../_bounds";

/** Shape of one location row in the left rail. Mirrors the future
 *  pkg/files location API — top-level "saved" folders the user has
 *  starred for quick navigation. Strings stay opaque ("4.2 GB",
 *  "180 MB") because the source-of-truth is the indexer's
 *  human-readable formatter. */
interface LocationRow {
  /** Folder name as shown to the user. */
  name:  string;
  /** Item count under this location. */
  count: number;
  /** Human-readable rolled-up size ("4.2 GB", "180 MB"). */
  size:  string;
  /** Lethean-managed location (brand-coloured, slight highlight). */
  brand?: boolean;
}

/** Shape of one recent-file row in the right pane. Mirrors the future
 *  pkg/files recent API — files modified in the last N days, sorted
 *  newest-first. */
interface RecentRow {
  /** File basename — e.g. "sow-heritage-law-v2.md". */
  name: string;
  /** Parent directory in display form (~ for $HOME). */
  path: string;
  /** Human-readable last-edit timestamp ("14:42", "yest", "3 d"). */
  when: string;
  /** Human-readable file size ("38 KB", "2.1 GB"). */
  size: string;
}

/** Shape of the disk-free summary at the bottom of the rail. */
interface DiskMeter {
  /** Human-readable free-space figure ("312 GB"). */
  free:  string;
  /** Human-readable total-disk figure ("1 TB"). */
  total: string;
  /** Fill percentage 0-100 — width of the brand bar. */
  used:  number;
}

const FIXTURE_LOCATIONS: LocationRow[] = [
  { name: "Code",         count: 124, size: "4.2 GB"  },
  { name: "Documents",    count: 62,  size: "180 MB"  },
  { name: "lthn / models", count: 6,  size: "14 GB", brand: true },
  { name: "Recordings",   count: 18,  size: "2.4 GB"  },
  { name: "Screenshots",  count: 248, size: "840 MB"  },
];

const FIXTURE_RECENT: RecentRow[] = [
  { name: "sow-heritage-law-v2.md",   path: "~/Documents/sales/",         when: "14:42", size: "38 KB"  },
  { name: "v0.2-release-notes.md",    path: "~/Code/lthn/docs/",          when: "14:18", size: "4.2 KB" },
  { name: "gemma-4-e2b-q4_k_m.gguf",  path: "~/.lthn/models/",            when: "yest",  size: "2.1 GB" },
  { name: "calliope-call-notes.md",   path: "~/Documents/investors/",     when: "yest",  size: "6.8 KB" },
  { name: "lthn-icon-helmet@2x.png",  path: "~/Code/lthn/desktop/assets/", when: "3 d",  size: "82 KB"  },
];

const FIXTURE_DISK: DiskMeter = { free: "312 GB", total: "1 TB", used: 68 };

class LthnViewFiles extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    selectedLocation: { state: true },
    locations: { state: true },
    recent:    { state: true },
    disk:      { state: true },
    loading:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare selectedLocation: string | null;
  declare locations: LocationRow[];
  declare recent: RecentRow[];
  declare disk: DiskMeter;
  declare loading: boolean;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.selectedLocation = null;
    this.locations = FIXTURE_LOCATIONS;
    this.recent = FIXTURE_RECENT;
    this.disk = FIXTURE_DISK;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend(): Promise<void> {
    if (this.loading) return;
    this.loading = true;
    try {
      const svc = await import("@desktop/office/files/service").catch(() => null);
      if (!svc || typeof (svc as { ListLocations?: unknown }).ListLocations !== "function") {
        return;
      }
      const filesSvc = svc as {
        ListLocations: () => Promise<{ Value?: { locations?: LocationRow[] } }>;
        ListRecent: (input: { locationName?: string; limit?: number }) => Promise<{ Value?: { files?: RecentRow[] } }>;
        GetDiskUsage: () => Promise<{ Value?: DiskMeter }>;
      };

      const [lr, rr, dr] = await Promise.all([
        filesSvc.ListLocations(),
        filesSvc.ListRecent({}),
        filesSvc.GetDiskUsage(),
      ]);

      // Mantis #1491 — defence-in-depth length cap on backend response.
      const locations = clampRows<LocationRow>(lr?.Value?.locations);
      if (locations.length > 0) {
        this.locations = locations;
      }

      const files = clampRows<RecentRow>(rr?.Value?.files);
      if (files.length > 0) {
        this.recent = files;
      }

      const disk = dr?.Value;
      if (disk && disk.free) {
        this.disk = disk;
      }
    } catch {
      // Binding unavailable — keep fixture data.
    } finally {
      this.loading = false;
    }
  }

  /** Toggle a location selection — clicking the active one clears it.
   *  Split out so tests can drive the state without a click event. */
  _selectLocation(name: string) {
    this.selectedLocation = (this.selectedLocation === name) ? null : name;
  }

  render() {
    const recent = this.recent;

    const body = html`
      <div class="lthn-view-files" style="flex:1; display:grid; grid-template-columns: 240px 1fr; min-height:0;">
        <!-- locations rail -->
        <aside class="lthn-view-files-locations" style="background:rgba(0,0,0,0.20); border-right:1px solid rgba(255,255,255,0.05); padding:14px 8px; display:flex; flex-direction:column; gap:1px;">
          <div style="padding:0 12px 6px; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Locations</div>
          ${this.locations.map(loc => {
            const active = this.selectedLocation === loc.name;
            return html`
              <div class="lthn-view-files-location"
                   data-name=${loc.name}
                   ?data-brand=${!!loc.brand}
                   ?data-active=${active}
                   @click=${() => this._selectLocation(loc.name)}
                   style="display:flex; align-items:center; gap:10px; padding:8px 12px; border-radius:6px;
                          ${loc.brand
                            ? "background:rgba(64,193,197,0.08); border:1px solid rgba(64,193,197,0.22);"
                            : active
                              ? "background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10);"
                              : "border:1px solid transparent;"}
                          color:${loc.brand ? "var(--brand-300)" : "var(--fg-2)"};
                          font-size:12.5px; cursor:pointer;
                          --wails-draggable: no-drag;">
                <i class="fa-regular fa-folder" style="font-size:11px; color:${loc.brand ? "var(--brand-300)" : "var(--fg-3)"};"></i>
                <span style="flex:1;">${loc.name}</span>
                <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${loc.count}</span>
              </div>
            `;
          })}
          <div style="height:1px; background:rgba(255,255,255,0.05); margin:8px 8px;"></div>
          <div class="lthn-view-files-disk" style="padding:0 12px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3); line-height:1.6;">
            Free<br>
            <span style="color:var(--fg-1); font-size:14px;">${this.disk.free}</span> of ${this.disk.total}
            <div style="height:4px; border-radius:2px; background:rgba(255,255,255,0.06); margin-top:6px; overflow:hidden;">
              <div style="width:${this.disk.used}%; height:100%; background:var(--brand-400);"></div>
            </div>
          </div>
        </aside>

        <!-- recent files pane -->
        <main class="lthn-view-files-recent" style="padding:18px 22px; overflow:auto;">
          <div style="display:flex; align-items:baseline; gap:12px; margin-bottom:10px;">
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Recent files</div>
            ${this.selectedLocation
              ? html`<span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);">· filtered: ${this.selectedLocation}</span>`
              : html`<span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">· all locations</span>`}
          </div>
          <div class="lthn-view-files-list" style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${recent.map((f, i) => html`
              <div class="lthn-view-files-row"
                   data-name=${f.name}
                   style="display:grid; grid-template-columns: 28px 1fr 1.2fr 80px 80px; gap:14px;
                          padding:12px 16px;
                          border-bottom:${i < recent.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          align-items:center;">
                <i class="fa-regular fa-file" style="font-size:13px; color:var(--fg-3);"></i>
                <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-0);">${f.name}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${f.path}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">${f.when}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2); text-align:right;">${f.size}</span>
              </div>
            `)}
          </div>
        </main>
      </div>
    `;

    return renderChrome({
      title: "Files",
      subtitle: `~/Documents · ${recent.length} recent`,
      w: this.w, h: this.h,
      body,
      footer: html`local filesystem · indexed by lthn · ⌘O to open · ⌘shift+P to find`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-files", LthnViewFiles);
