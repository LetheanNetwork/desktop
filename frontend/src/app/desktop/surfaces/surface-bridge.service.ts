import { Injectable } from '@angular/core';

interface ResultEnvelope {
  readonly OK?: boolean;
  readonly ok?: boolean;
  readonly Value?: unknown;
  readonly value?: unknown;
  readonly Error?: unknown;
  readonly error?: unknown;
}

@Injectable({ providedIn: 'root' })
export class SurfaceBridgeService {
  async call(method: string, args: readonly unknown[] = []): Promise<unknown> {
    const { Call } = await import('@wailsio/runtime');
    const controller = new AbortController();
    const raw = await Call.ByName(method, ...args).cancelOn(controller.signal);
    return unwrapResult(raw);
  }

  async request(input: RequestInfo | URL, init?: RequestInit): Promise<unknown> {
    const response = await fetch(input, init);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    const contentType = response.headers.get('content-type') ?? '';
    if (!contentType.includes('json')) return response.text();
    const raw = (await response.json()) as unknown;
    if (raw && typeof raw === 'object' && 'data' in raw) {
      return (raw as { data: unknown }).data;
    }
    return unwrapResult(raw);
  }
}

export function unwrapResult(raw: unknown): unknown {
  if (!raw || typeof raw !== 'object') return raw;
  const envelope = raw as ResultEnvelope;
  if (typeof envelope.OK !== 'boolean' && typeof envelope.ok !== 'boolean') {
    return raw;
  }
  const ok = envelope.OK ?? envelope.ok ?? false;
  if (ok) {
    return Object.hasOwn(envelope, 'Value') ? envelope.Value : envelope.value;
  }
  const failure = Object.hasOwn(envelope, 'Error') ? envelope.Error : envelope.error;
  throw new Error(resultMessage(failure ?? envelope.Value ?? envelope.value));
}

function resultMessage(value: unknown): string {
  if (value instanceof Error) return value.message;
  if (typeof value === 'string' && value) return value;
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    for (const key of ['message', 'Message', 'error', 'Error']) {
      if (typeof record[key] === 'string' && record[key]) return record[key];
    }
  }
  return 'The desktop service rejected the request.';
}
