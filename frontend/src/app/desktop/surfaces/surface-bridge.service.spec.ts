const wailsHarness = vi.hoisted(() => ({
  byName: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: wailsHarness.byName },
}));

import { SurfaceBridgeService, unwrapResult } from './surface-bridge.service';

describe('SurfaceBridgeService.call', () => {
  let service: SurfaceBridgeService;

  beforeEach(() => {
    service = new SurfaceBridgeService();
    wailsHarness.byName.mockReset();
  });

  it('calls the named Wails method and unwraps its Result envelope', async () => {
    wailsHarness.byName.mockReturnValue({
      cancelOn: () => Promise.resolve({ OK: true, Value: { rows: 3 } }),
    });

    await expect(service.call('pkg.Service.Method', [1, 2])).resolves.toEqual({ rows: 3 });
    expect(wailsHarness.byName).toHaveBeenCalledWith('pkg.Service.Method', 1, 2);
  });

  it('propagates a Wails Result failure envelope as an error', async () => {
    wailsHarness.byName.mockReturnValue({
      cancelOn: () => Promise.resolve({ OK: false, Error: 'not unlocked' }),
    });

    await expect(service.call('pkg.Service.Method')).rejects.toThrow('not unlocked');
  });
});

describe('SurfaceBridgeService.request', () => {
  let service: SurfaceBridgeService;
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    service = new SurfaceBridgeService();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('throws with the HTTP status when the response is not ok', async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue({ ok: false, status: 503, headers: new Headers() });

    await expect(service.request('/x')).rejects.toThrow('HTTP 503');
  });

  it('returns plain text for a non-JSON content type', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'text/plain' }),
      text: () => Promise.resolve('hello'),
    });

    await expect(service.request('/x')).resolves.toBe('hello');
  });

  it('unwraps a JSON body already shaped as { data }', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: () => Promise.resolve({ data: { total: 5 } }),
    });

    await expect(service.request('/x')).resolves.toEqual({ total: 5 });
  });

  it('unwraps a bare Core Result JSON body', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: () => Promise.resolve({ OK: true, Value: [1, 2] }),
    });

    await expect(service.request('/x')).resolves.toEqual([1, 2]);
  });
});

describe('surface result bridge', () => {
  it('unwraps Core Result success envelopes in both supported casings', () => {
    expect(unwrapResult({ OK: true, Value: { rows: 2 } })).toEqual({ rows: 2 });
    expect(unwrapResult({ ok: true, value: 'ready' })).toBe('ready');
  });

  it('turns Core Result failures into useful errors', () => {
    expect(() => unwrapResult({ OK: false, Error: { message: 'not unlocked' } })).toThrow(
      'not unlocked',
    );
    expect(() => unwrapResult({ ok: false })).toThrow('The desktop service rejected the request.');
  });

  it('does not reshape non-Result payloads', () => {
    const value = { data: [1, 2] };
    expect(unwrapResult(value)).toBe(value);
    expect(unwrapResult('plain')).toBe('plain');
  });
});
