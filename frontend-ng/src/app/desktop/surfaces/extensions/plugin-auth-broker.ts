export const AUTH_GRANT_TYPE = 'lthn:auth:grant';
export const CAPABILITY_GRANT_AUDIT_PATH = '/v1/plugin-view/capability-grant';

export interface GrantTargetFrame {
  readonly source: Window;
  readonly targetOrigin: string;
  readonly pluginCode: string;
}

export type GrantOutcome =
  | { readonly ok: true; readonly scopes: string[] }
  | {
      readonly ok: false;
      readonly reason: 'audit-failed' | 'no-session' | 'postmessage-failed';
    };

// Session credentials intentionally live only in this module's closure. They
// are never placed in signals, NgRx, web storage, component inputs, or events.
let cachedSessionToken: string | null = null;

export function setPluginSessionToken(token: string): void {
  cachedSessionToken = token.trim() ? token : null;
}

export function clearPluginSessionToken(): void {
  cachedSessionToken = null;
}

/**
 * The only plugin credential-release path. A server-side audit row is
 * committed before the browser is asked to deliver the grant to the verified
 * frame, and the outcome never contains credential bytes.
 */
export async function grantTokenToFrame(
  target: GrantTargetFrame,
  scopes: readonly string[],
): Promise<GrantOutcome> {
  const token = cachedSessionToken;
  if (!token) return { ok: false, reason: 'no-session' };

  let auditOk = false;
  try {
    const response = await fetch(CAPABILITY_GRANT_AUDIT_PATH, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        plugin_id: target.pluginCode,
        capabilities: [...scopes],
        origin: target.targetOrigin,
        outcome: 'granted',
      }),
    });
    auditOk = response.ok;
  } catch {
    auditOk = false;
  }
  if (!auditOk) return { ok: false, reason: 'audit-failed' };

  const payload = {
    type: AUTH_GRANT_TYPE,
    token,
    scopes: [...scopes],
  };
  try {
    target.source.postMessage(payload, target.targetOrigin);
  } catch {
    return { ok: false, reason: 'postmessage-failed' };
  }
  return { ok: true, scopes: [...scopes] };
}
