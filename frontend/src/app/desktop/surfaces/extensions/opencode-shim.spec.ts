import { PluginViewDescriptor } from './plugin-view-runtime';
import {
  AUTH_REQUEST_TYPE,
  DENY_REASON_CAPABILITY_NOT_DECLARED,
  DENY_REASON_NO_ORIGIN,
  DENY_REASON_ORIGIN_MISMATCH,
  DENY_REASON_SOURCE_MISMATCH,
  DENY_REASON_UNKNOWN_SCOPE,
  decideAuthRequest,
  expectedPluginOrigin,
} from './opencode-shim';

const frame = {} as Window;
const descriptor: PluginViewDescriptor = {
  id: 'opencode',
  label: 'OpenCode',
  icon: 'terminal',
  group: 'plugin',
  kind: 'iframe',
  source: 'http://127.0.0.1:4096',
  pluginCode: 'opencode',
  capabilities: ['session-token'],
  loopbackOrigin: 'http://127.0.0.1:4096',
};

describe('OpenCode auth request decision', () => {
  it('grants only a declared session-token request from the exact frame and origin', () => {
    expect(
      decideAuthRequest(descriptor, frame, frame, 'http://127.0.0.1:4096', {
        type: AUTH_REQUEST_TYPE,
        scopes: ['session-token'],
      }),
    ).toEqual({
      kind: 'grant',
      scopes: ['session-token'],
      origin: 'http://127.0.0.1:4096',
    });
  });

  it.each([
    [
      'wrong source',
      descriptor,
      {} as Window,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_SOURCE_MISMATCH,
    ],
    [
      'missing origin',
      { ...descriptor, loopbackOrigin: '' },
      frame,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_NO_ORIGIN,
    ],
    [
      'wrong origin',
      descriptor,
      frame,
      'http://127.0.0.1:9999',
      ['session-token'],
      DENY_REASON_ORIGIN_MISMATCH,
    ],
    [
      'undeclared scope',
      { ...descriptor, capabilities: [] },
      frame,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_CAPABILITY_NOT_DECLARED,
    ],
    [
      'unknown forward scope',
      { ...descriptor, capabilities: ['vi-events'] },
      frame,
      'http://127.0.0.1:4096',
      ['vi-events'],
      DENY_REASON_UNKNOWN_SCOPE,
    ],
  ])('denies %s', (_label, desc, source, origin, scopes, reason) => {
    expect(
      decideAuthRequest(desc, frame, source, origin, {
        type: AUTH_REQUEST_TYPE,
        scopes,
      }),
    ).toEqual({ kind: 'deny', reason });
  });

  it('ignores unrelated messages and requests without a descriptor', () => {
    expect(decideAuthRequest(descriptor, frame, frame, '', { type: 'other' })).toEqual({
      kind: 'ignore',
    });
    expect(
      decideAuthRequest(null, frame, frame, '', {
        type: AUTH_REQUEST_TYPE,
        scopes: ['session-token'],
      }),
    ).toEqual({ kind: 'ignore' });
  });

  it('uses the desktop origin for same-origin proxied plugin views', () => {
    expect(
      expectedPluginOrigin(
        { ...descriptor, loopbackOrigin: undefined, proxied: true },
        'http://127.0.0.1:9245',
      ),
    ).toBe('http://127.0.0.1:9245');
  });
});
