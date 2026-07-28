import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { SurfaceRoute } from '../surface-page';
import { PluginViewDescriptor, PluginViewFrame } from './plugin-view-runtime';

@Component({
  selector: 'lthn-extensions-plugin-view-surface',
  imports: [PluginViewFrame],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="plugin-view">
      <header>
        <span class="icon"><i class="fa-solid fa-puzzle-piece" aria-hidden="true"></i></span>
        <div>
          <div class="eyebrow" i18n="Extensions eyebrow@@surface.extensions.eyebrow">
            Extensions
          </div>
          <h2 i18n="Plugin view title@@surface.extensions.pluginView.title">Plugin view</h2>
          <p i18n="Plugin view subtitle@@surface.extensions.pluginView.subtitle">
            Sandboxed plugin content resolved from the installed marketplace manifest.
          </p>
        </div>
        <label>
          <span i18n="Plugin view id label@@surface.extensions.pluginView.idLabel">View id</span>
          <input
            [value]="viewId()"
            (change)="changeView($event)"
            aria-label="Plugin view id"
            i18n-aria-label="Plugin view id aria label@@surface.extensions.pluginView.idAria"
          />
        </label>
      </header>

      @if (descriptor()) {
        <div class="descriptor">
          <span>{{ descriptor()!.pluginCode }}</span>
          <span>{{ descriptor()!.kind }}</span>
          <span>{{ descriptor()!.capabilities?.join(', ') || 'no account capabilities' }}</span>
        </div>
      }

      <lthn-plugin-view-frame [viewId]="viewId()" (descriptorLoaded)="descriptor.set($event)" />
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
    }
    .plugin-view {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      gap: 12px;
      padding: 20px;
      overflow: hidden;
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
    label {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--fg-3);
      font: 9px var(--font-mono);
    }
    input {
      width: 150px;
      padding: 7px 9px;
      color: var(--fg-0);
      background: var(--ink-2);
      border: 1px solid var(--line-1);
      border-radius: 6px;
      font: 10px var(--font-mono);
    }
    .descriptor {
      display: flex;
      gap: 7px;
    }
    .descriptor span {
      padding: 3px 6px;
      color: var(--fg-2);
      border: 1px solid var(--line-1);
      border-radius: 999px;
      font: 8px var(--font-mono);
    }
    @media (max-width: 650px) {
      header {
        align-items: flex-start;
        flex-wrap: wrap;
      }
      label {
        width: 100%;
      }
    }
  `,
})
export class ExtensionsPluginViewSurface extends SurfaceRoute {
  readonly viewId = signal('opencode');
  readonly descriptor = signal<PluginViewDescriptor | null>(null);

  changeView(event: Event): void {
    const value = (event.target as HTMLInputElement).value.trim();
    if (!value || value === this.viewId()) return;
    this.descriptor.set(null);
    this.viewId.set(value);
  }
}
