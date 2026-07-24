import {
  ChangeDetectionStrategy,
  Component,
  OnDestroy,
  OnInit,
  ViewChild,
  signal,
} from '@angular/core';
import { SurfaceRoute } from '../surface-page';
import { grantTokenToFrame } from './plugin-auth-broker';
import { PluginViewDescriptor, PluginViewFrame } from './plugin-view-runtime';

export const AUTH_REQUEST_TYPE = 'lthn:auth:request';
export const AUTH_DENY_TYPE = 'lthn:auth:deny';
export const AUTH_GRANTED_EVENT = 'lthn:plugin-view:auth-granted';
export const AUTH_DENIED_EVENT = 'lthn:plugin-view:auth-denied';
export const CAPABILITY_SESSION_TOKEN = 'session-token';

export const DENY_REASON_AUDIT_FAILED = 'audit-failed';
export const DENY_REASON_NO_ORIGIN = 'no-origin';
export const DENY_REASON_ORIGIN_MISMATCH = 'origin-mismatch';
export const DENY_REASON_SOURCE_MISMATCH = 'source-mismatch';
export const DENY_REASON_NO_SESSION = 'no-session';
export const DENY_REASON_CAPABILITY_NOT_DECLARED = 'capability-not-declared';
export const DENY_REASON_UNKNOWN_SCOPE = 'unknown-scope';

export type AuthRequestDecision =
  | { readonly kind: 'ignore' }
  | { readonly kind: 'deny'; readonly reason: string }
  | { readonly kind: 'grant'; readonly scopes: string[]; readonly origin: string };

export function expectedPluginOrigin(
  descriptor: PluginViewDescriptor,
  hostOrigin = window.location.origin,
): string {
  const loopbackOrigin = (descriptor.loopbackOrigin ?? '').trim();
  return loopbackOrigin || (descriptor.proxied ? hostOrigin : '');
}

export function decideAuthRequest(
  descriptor: PluginViewDescriptor | null,
  frameWindow: Window | null,
  source: MessageEventSource | null,
  origin: string,
  data: unknown,
): AuthRequestDecision {
  if (!descriptor || !data || typeof data !== 'object') return { kind: 'ignore' };
  const request = data as { type?: unknown; scopes?: unknown };
  if (request.type !== AUTH_REQUEST_TYPE) return { kind: 'ignore' };
  if (!frameWindow || source !== frameWindow) {
    return { kind: 'deny', reason: DENY_REASON_SOURCE_MISMATCH };
  }

  const expectedOrigin = expectedPluginOrigin(descriptor);
  if (!expectedOrigin) return { kind: 'deny', reason: DENY_REASON_NO_ORIGIN };
  if (origin !== expectedOrigin) {
    return { kind: 'deny', reason: DENY_REASON_ORIGIN_MISMATCH };
  }

  const requestedScopes = Array.isArray(request.scopes)
    ? request.scopes.filter((scope): scope is string => typeof scope === 'string')
    : [];
  if (!requestedScopes.length) {
    return { kind: 'deny', reason: DENY_REASON_CAPABILITY_NOT_DECLARED };
  }
  const declared = descriptor.capabilities ?? [];
  for (const scope of requestedScopes) {
    if (!declared.includes(scope)) {
      return { kind: 'deny', reason: DENY_REASON_CAPABILITY_NOT_DECLARED };
    }
    if (scope !== CAPABILITY_SESSION_TOKEN) {
      return { kind: 'deny', reason: DENY_REASON_UNKNOWN_SCOPE };
    }
  }
  return { kind: 'grant', scopes: requestedScopes, origin: expectedOrigin };
}

@Component({
  selector: 'lthn-extensions-opencode-shim-surface',
  imports: [PluginViewFrame],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="shim">
      <header>
        <span class="icon"><i class="fa-solid fa-terminal" aria-hidden="true"></i></span>
        <div>
          <div class="eyebrow" i18n="Extensions eyebrow@@surface.extensions.eyebrow">
            Extensions
          </div>
          <h2 i18n="OpenCode title@@surface.extensions.opencode.title">OpenCode</h2>
          <p i18n="OpenCode subtitle@@surface.extensions.opencode.subtitle">
            Isolated coding workspace with verified-origin account capability brokering.
          </p>
        </div>
        <span class="security">
          <i class="fa-solid fa-shield-halved" aria-hidden="true"></i>
          <span i18n="OpenCode security state@@surface.extensions.opencode.security"
            >sandboxed · audited grants</span
          >
        </span>
      </header>

      @if (status()) {
        <div class="status" role="status">{{ status() }}</div>
      }

      <lthn-plugin-view-frame #frame viewId="opencode" (descriptorLoaded)="onDescriptor($event)" />
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
    }
    .shim {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      gap: 11px;
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
    .security,
    .status {
      color: var(--brand-200);
      font: 9px var(--font-mono);
    }
    .security i {
      margin-right: 5px;
    }
    .status {
      padding: 7px 9px;
      background: rgba(64, 193, 197, 0.06);
      border: 1px solid rgba(64, 193, 197, 0.17);
      border-radius: 6px;
    }
  `,
})
export class ExtensionsOpencodeShimSurface extends SurfaceRoute implements OnInit, OnDestroy {
  @ViewChild('frame') frame?: PluginViewFrame;

  readonly status = signal('');
  private descriptor: PluginViewDescriptor | null = null;

  ngOnInit(): void {
    window.addEventListener('message', this.onMessage);
  }

  ngOnDestroy(): void {
    window.removeEventListener('message', this.onMessage);
  }

  onDescriptor(descriptor: PluginViewDescriptor): void {
    this.descriptor = descriptor;
  }

  private readonly onMessage = (event: MessageEvent): void => {
    const decision = decideAuthRequest(
      this.descriptor,
      this.frame?.iframeWindow ?? null,
      event.source,
      event.origin,
      event.data,
    );
    if (decision.kind === 'ignore') return;
    if (decision.kind === 'deny') {
      this.deny(event, decision.reason);
      return;
    }
    void this.broker(event, decision.scopes, decision.origin);
  };

  private async broker(event: MessageEvent, scopes: string[], targetOrigin: string): Promise<void> {
    const source = event.source as Window | null;
    if (!source || typeof source.postMessage !== 'function') return;
    const outcome = await grantTokenToFrame(
      {
        source,
        targetOrigin,
        pluginCode: this.descriptor?.pluginCode ?? '',
      },
      scopes,
    );
    if (!outcome.ok) {
      this.deny(
        event,
        outcome.reason === 'audit-failed' ? DENY_REASON_AUDIT_FAILED : DENY_REASON_NO_SESSION,
      );
      return;
    }
    const detail = {
      viewId: this.descriptor?.id ?? '',
      pluginCode: this.descriptor?.pluginCode ?? '',
      scopes: outcome.scopes,
    };
    this.status.set(
      $localize`:OpenCode access grant status@@surface.extensions.opencode.granted:Account access granted through the audited broker.`,
    );
    window.dispatchEvent(new CustomEvent(AUTH_GRANTED_EVENT, { detail }));
  }

  private deny(event: MessageEvent, reason: string): void {
    const expectedOrigin = this.descriptor ? expectedPluginOrigin(this.descriptor) : '';
    const source = event.source as Window | null;
    if (source && typeof source.postMessage === 'function' && expectedOrigin) {
      try {
        source.postMessage({ type: AUTH_DENY_TYPE, reason }, expectedOrigin);
      } catch {
        // The target frame can disappear between validation and response.
      }
    }
    const detail = {
      viewId: this.descriptor?.id ?? '',
      pluginCode: this.descriptor?.pluginCode ?? '',
      reason,
    };
    this.status.set(
      $localize`:OpenCode access denial status@@surface.extensions.opencode.denied:Account access request denied (${reason}:reason:).`,
    );
    window.dispatchEvent(new CustomEvent(AUTH_DENIED_EVENT, { detail }));
  }
}
