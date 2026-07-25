// SPDX-Licence-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  developmentCommandEnvironment,
  runDevelopmentCommand,
} from './wails-dev-command.mjs';

const launcherPath = fileURLToPath(new URL('./wails-dev-command.mjs', import.meta.url));
const managedKeys = [
  'EXTRA_TAGS',
  'LTHN_DEV',
  'LTHN_WAILS_WS_LISTEN',
  'LTHN_WAILS_WS_URL',
];
const managedKeySet = new Set(managedKeys.map((key) => key.toLowerCase()));

function managedEntries(environment) {
  return Object.entries(environment)
    .filter(([key]) => managedKeySet.has(key.toLowerCase()))
    .map(([key, value]) => [key.toUpperCase(), value])
    .sort(([left], [right]) => left.localeCompare(right));
}

const buildManagedEntries = [['EXTRA_TAGS', 'mcp']];
const runManagedEntries = [
  ['LTHN_DEV', '1'],
  ['LTHN_WAILS_WS_LISTEN', '127.0.0.1:9199'],
  ['LTHN_WAILS_WS_URL', 'ws://localhost:9199/wails/ws'],
];

const hostileAmbient = Object.freeze({
  PATH: '/example/bin',
  UNMANAGED_VALUE: 'preserved',
  EXTRA_TAGS: 'ambient-build',
  extra_tags: 'lower-build',
  LTHN_DEV: 'ambient-dev',
  lthn_dev: 'lower-dev',
  LTHN_WAILS_WS_LISTEN: '127.0.0.1:7777',
  lthn_wails_ws_listen: '127.0.0.1:6666',
  LTHN_WAILS_WS_URL: 'ws://localhost:7777/wails/ws',
  lthn_wails_ws_url: 'ws://localhost:6666/wails/ws',
});

test('development command environments replace every managed ambient key exactly', () => {
  const build = developmentCommandEnvironment('build', hostileAmbient);
  assert.deepEqual(managedEntries(build), buildManagedEntries);
  assert.equal(build.UNMANAGED_VALUE, 'preserved');

  const run = developmentCommandEnvironment('run', hostileAmbient);
  assert.deepEqual(managedEntries(run), runManagedEntries);
  assert.equal(run.UNMANAGED_VALUE, 'preserved');
});

test('launcher passes the exact native child boundary under hostile ambient values', () => {
  for (const [command, expectedManaged] of [
    ['build', buildManagedEntries],
    ['run', runManagedEntries],
  ]) {
    let captured;
    const status = runDevelopmentCommand(
      command,
      (executable, args, options) => {
        captured = { executable, args, options };
        return { status: 0 };
      },
      hostileAmbient,
    );

    assert.equal(status, 0);
    assert.equal(captured.executable, 'wails3');
    assert.deepEqual(captured.args, ['task', command]);
    assert.equal(captured.options.stdio, 'inherit');
    assert.equal(Object.hasOwn(captured.options, 'shell'), false);
    assert.deepEqual(managedEntries(captured.options.env), expectedManaged);
    assert.equal(captured.options.env.PATH, '/example/bin');
    assert.equal(captured.options.env.UNMANAGED_VALUE, 'preserved');
  }
});

test('runDevelopmentCommand propagates launch errors and child status', () => {
  const launchError = new Error('cannot launch wails3');
  assert.throws(
    () => runDevelopmentCommand('build', () => ({ error: launchError, status: null })),
    (error) => error === launchError,
  );
  assert.equal(runDevelopmentCommand('run', () => ({ status: 17 })), 17);
});

test('launcher rejects unknown commands', () => {
  assert.throws(
    () => developmentCommandEnvironment('deploy', {}),
    /unknown development command: deploy/,
  );

  const result = spawnSync(process.execPath, [launcherPath, 'deploy'], {
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unknown development command: deploy/);
});
