import { unwrapResult } from './surface-bridge.service';

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
