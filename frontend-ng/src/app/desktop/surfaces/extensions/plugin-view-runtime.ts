import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ElementRef,
  EventEmitter,
  Input,
  OnChanges,
  OnDestroy,
  OnInit,
  Output,
  SimpleChanges,
  ViewChild,
  inject,
  signal,
} from '@angular/core';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { SurfaceBridgeService } from '../surface-bridge.service';

export interface PluginViewDescriptor {
  readonly id: string;
  readonly label: string;
  readonly icon: string;
  readonly group: 'plugin';
  readonly kind: 'iframe' | 'lit';
  readonly source: string;
  readonly pluginCode: string;
  readonly capabilities?: readonly string[];
  readonly loopbackPort?: number;
  readonly loopbackOrigin?: string;
  readonly proxied?: boolean;
}

export const DEFAULT_PLUGIN_IFRAME_SANDBOX = 'allow-scripts allow-forms allow-same-origin';
export const PLUGIN_VIEW_MOUNT_TIMEOUT_MS = 1_500;
export const PLUGIN_VIEW_MOUNT_HARD_CAP_MS = 3_000;
export const PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT = 'lthn:plugin-view:mount-timeout';
export const PLUGIN_VIEW_REVOKED_EVENT = 'lthn:plugin-view:revoked';

const MARKETPLACE_SERVICE = 'dappco.re/lthn/desktop/pkg/marketplace.Service';

@Component({
  selector: 'lthn-plugin-lit-outlet',
  template: '',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PluginLitOutlet implements AfterViewInit, OnChanges {
  private readonly host = inject(ElementRef<HTMLElement>);
  private ready = false;

  @Input({ required: true }) descriptor!: PluginViewDescriptor;

  ngAfterViewInit(): void {
    this.ready = true;
    this.renderElement();
  }

  ngOnChanges(): void {
    if (this.ready) this.renderElement();
  }

  private renderElement(): void {
    const host = this.host.nativeElement;
    host.replaceChildren();
    const tag = this.descriptor?.source.trim();
    if (!tag || !customElements.get(tag)) return;
    const element = document.createElement(tag);
    element.setAttribute('embedded', '');
    element.setAttribute('plugin-code', this.descriptor.pluginCode);
    host.append(element);
  }
}

@Component({
  selector: 'lthn-plugin-view-frame',
  imports: [PluginLitOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="plugin-frame" [attr.data-state]="state()">
      @if (loading()) {
        <div class="plugin-state">
          <i class="fa-solid fa-spinner fa-spin" aria-hidden="true"></i>
          <span i18n="Plugin view loading@@surface.extensions.plugin.loading">Loading view…</span>
        </div>
      } @else if (timedOut()) {
        <div class="plugin-state plugin-state--error" role="alert">
          <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
          <span i18n="Plugin mount failure@@surface.extensions.plugin.mountFailure"
            >Plugin view failed to mount within 1,500 ms.</span
          >
        </div>
      } @else if (!current()) {
        <div class="plugin-state">
          <i class="fa-solid fa-puzzle-piece" aria-hidden="true"></i>
          <span i18n="Plugin descriptor missing@@surface.extensions.plugin.notFound"
            >No installed plugin view matches this id.</span
          >
        </div>
      } @else if (current()!.kind === 'iframe' && safeSource()) {
        <iframe
          #iframe
          sandbox="allow-scripts allow-forms allow-same-origin"
          [src]="safeSource()"
          [title]="current()!.label"
          (load)="onIframeLoad()"
        ></iframe>
      } @else if (current()!.kind === 'lit' && litElementAvailable()) {
        <lthn-plugin-lit-outlet [descriptor]="current()!" />
      } @else {
        <div class="plugin-state plugin-state--error" role="alert">
          <i class="fa-solid fa-shield-halved" aria-hidden="true"></i>
          <span>{{ sourceError() }}</span>
        </div>
      }
    </section>
  `,
  styles: `
    :host {
      display: flex;
      flex: 1;
      min-height: 0;
    }
    .plugin-frame {
      display: flex;
      flex: 1;
      min-width: 0;
      min-height: 280px;
      overflow: hidden;
      background: #080c10;
      border: 1px solid var(--line-1);
      border-radius: 10px;
    }
    iframe,
    lthn-plugin-lit-outlet {
      display: block;
      flex: 1;
      width: 100%;
      min-height: 0;
      border: 0;
    }
    .plugin-state {
      display: flex;
      flex: 1;
      align-items: center;
      justify-content: center;
      gap: 9px;
      padding: 40px;
      color: var(--fg-3);
      font: 11px var(--font-mono);
      text-align: center;
    }
    .plugin-state i {
      color: var(--brand-300);
    }
    .plugin-state--error i {
      color: var(--warning-400);
    }
  `,
})
export class PluginViewFrame implements OnInit, OnChanges, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private readonly sanitizer = inject(DomSanitizer);
  private timeoutHandle: ReturnType<typeof setTimeout> | null = null;
  private inputDescriptor: PluginViewDescriptor | null = null;

  @Input() viewId = '';
  @Input() descriptor: PluginViewDescriptor | null = null;
  @Output() readonly descriptorLoaded = new EventEmitter<PluginViewDescriptor>();
  @Output() readonly mountTimeout = new EventEmitter<{
    viewId: string;
    pluginCode: string;
  }>();
  @ViewChild('iframe') iframe?: ElementRef<HTMLIFrameElement>;

  readonly current = signal<PluginViewDescriptor | null>(null);
  readonly loading = signal(true);
  readonly timedOut = signal(false);
  readonly safeSource = signal<SafeResourceUrl | null>(null);
  readonly sourceError = signal('');
  readonly state = signal<'loading' | 'ready' | 'missing' | 'timed-out' | 'invalid'>('loading');

  get iframeWindow(): Window | null {
    return this.iframe?.nativeElement.contentWindow ?? null;
  }

  ngOnInit(): void {
    void this.hydrate();
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['descriptor']) {
      this.inputDescriptor = this.descriptor;
      if (this.descriptor) {
        this.useDescriptor(this.descriptor);
        return;
      }
    }
    if (changes['viewId'] && !changes['viewId'].firstChange) {
      this.current.set(null);
      this.safeSource.set(null);
      this.loading.set(true);
      this.state.set('loading');
      void this.hydrate();
    }
  }

  ngOnDestroy(): void {
    this.clearMountTimeout();
  }

  onIframeLoad(): void {
    this.clearMountTimeout();
    this.timedOut.set(false);
    this.state.set('ready');
  }

  litElementAvailable(): boolean {
    const descriptor = this.current();
    return (
      !!descriptor && descriptor.kind === 'lit' && !!customElements.get(descriptor.source.trim())
    );
  }

  private async hydrate(): Promise<void> {
    if (this.inputDescriptor ?? this.descriptor) {
      this.useDescriptor((this.inputDescriptor ?? this.descriptor)!);
      return;
    }
    const id = this.viewId.trim();
    if (!id) {
      this.loading.set(false);
      this.state.set('missing');
      return;
    }
    try {
      const value = await this.bridge.call(`${MARKETPLACE_SERVICE}.GetViewDescriptor`, [id]);
      const descriptor = normaliseDescriptor(value);
      if (descriptor) {
        this.useDescriptor(descriptor);
        return;
      }
    } catch {
      // The route remains useful in a browser preview; absence is rendered
      // explicitly rather than replacing the view with fixture credentials.
    }
    this.loading.set(false);
    this.current.set(null);
    this.state.set('missing');
  }

  private useDescriptor(descriptor: PluginViewDescriptor): void {
    this.clearMountTimeout();
    this.loading.set(false);
    this.timedOut.set(false);
    this.current.set(descriptor);
    this.descriptorLoaded.emit(descriptor);

    if (descriptor.kind === 'iframe') {
      const source = validatedPluginSource(descriptor);
      if (!source) {
        this.safeSource.set(null);
        this.sourceError.set(
          $localize`:Invalid plugin source@@surface.extensions.plugin.invalidSource:Plugin source failed the loopback-origin policy.`,
        );
        this.state.set('invalid');
        return;
      }
      this.safeSource.set(this.sanitizer.bypassSecurityTrustResourceUrl(source));
      this.state.set('loading');
      this.scheduleMountTimeout();
      return;
    }

    this.safeSource.set(null);
    if (this.litElementAvailable()) {
      this.state.set('ready');
    } else {
      this.sourceError.set(
        $localize`:Missing integrated plugin element@@surface.extensions.plugin.elementMissing:The integrated plugin element is not registered.`,
      );
      this.state.set('invalid');
      this.scheduleMountTimeout();
    }
  }

  private scheduleMountTimeout(timeout = PLUGIN_VIEW_MOUNT_TIMEOUT_MS): void {
    const duration = Math.min(timeout, PLUGIN_VIEW_MOUNT_HARD_CAP_MS);
    this.timeoutHandle = setTimeout(() => {
      this.timeoutHandle = null;
      if (this.state() === 'ready') return;
      this.timedOut.set(true);
      this.state.set('timed-out');
      const detail = {
        viewId: this.current()?.id ?? this.viewId,
        pluginCode: this.current()?.pluginCode ?? '',
      };
      this.mountTimeout.emit(detail);
      window.dispatchEvent(new CustomEvent(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, { detail }));
    }, duration);
  }

  private clearMountTimeout(): void {
    if (this.timeoutHandle) clearTimeout(this.timeoutHandle);
    this.timeoutHandle = null;
  }
}

export function normaliseDescriptor(value: unknown): PluginViewDescriptor | null {
  if (!value || typeof value !== 'object') return null;
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate['id'] !== 'string' ||
    typeof candidate['label'] !== 'string' ||
    typeof candidate['source'] !== 'string' ||
    typeof candidate['pluginCode'] !== 'string' ||
    (candidate['kind'] !== 'iframe' && candidate['kind'] !== 'lit')
  ) {
    return null;
  }
  return {
    id: candidate['id'],
    label: candidate['label'],
    icon: typeof candidate['icon'] === 'string' ? candidate['icon'] : 'puzzle-piece',
    group: 'plugin',
    kind: candidate['kind'],
    source: candidate['source'],
    pluginCode: candidate['pluginCode'],
    capabilities: Array.isArray(candidate['capabilities'])
      ? candidate['capabilities'].filter((value): value is string => typeof value === 'string')
      : [],
    loopbackPort:
      typeof candidate['loopbackPort'] === 'number' ? candidate['loopbackPort'] : undefined,
    loopbackOrigin:
      typeof candidate['loopbackOrigin'] === 'string' ? candidate['loopbackOrigin'] : undefined,
    proxied: candidate['proxied'] === true,
  };
}

export function validatedPluginSource(descriptor: PluginViewDescriptor): string {
  const source = descriptor.source.trim();
  if (!source || descriptor.kind !== 'iframe') return '';
  if (descriptor.proxied && source.startsWith('/v1/api/plugin/')) return source;

  try {
    const url = new URL(source);
    const expected = new URL(descriptor.loopbackOrigin ?? source);
    const loopback = url.hostname === '127.0.0.1' || url.hostname === 'localhost';
    if (!loopback || url.origin !== expected.origin) return '';
    return url.href;
  } catch {
    return '';
  }
}
