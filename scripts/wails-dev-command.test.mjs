// SPDX-Licence-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { chmod, mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
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

test('development command environments replace every managed ambient key exactly', () => {
  const hostileAmbient = {
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
  };

  const build = developmentCommandEnvironment('build', hostileAmbient);
  assert.deepEqual(managedEntries(build), buildManagedEntries);
  assert.equal(build.UNMANAGED_VALUE, 'preserved');

  const run = developmentCommandEnvironment('run', hostileAmbient);
  assert.deepEqual(managedEntries(run), runManagedEntries);
  assert.equal(run.UNMANAGED_VALUE, 'preserved');
});

async function writeFakeWails(binDir) {
  const shimSource = `
const { writeFileSync } = require('node:fs');
const managedKeys = new Set(${JSON.stringify([...managedKeySet])});
const managed = Object.entries(process.env)
  .filter(([key]) => managedKeys.has(key.toLowerCase()))
  .map(([key, value]) => [key.toUpperCase(), value])
  .sort(([left], [right]) => left.localeCompare(right));
writeFileSync(
  process.env.WAILS_CAPTURE_FILE,
  JSON.stringify({ args: process.argv.slice(2), managed }),
);
`;

  if (process.platform === 'win32') {
    const nodeShim = join(binDir, 'wails3-shim.cjs');
    await writeFile(nodeShim, shimSource);
    await writeFile(
      join(binDir, 'wails3.cmd'),
      '@echo off\r\n"%WAILS_TEST_NODE%" "%WAILS_TEST_SHIM%" %*\r\n',
    );
    return {
      WAILS_TEST_NODE: process.execPath,
      WAILS_TEST_SHIM: nodeShim,
    };
  }

  const executable = join(binDir, 'wails3');
  await writeFile(executable, `#!/usr/bin/env node\n${shimSource}`);
  await chmod(executable, 0o755);
  return {};
}

test('launcher passes exact arguments and managed environment under hostile ambient values', async () => {
  const binDir = await mkdtemp(join(tmpdir(), 'lthn-wails-command-'));
  const shimEnvironment = await writeFakeWails(binDir);

  for (const [command, expectedManaged] of [
    ['build', buildManagedEntries],
    ['run', runManagedEntries],
  ]) {
    const captureFile = join(binDir, `${command}.json`);
    const result = spawnSync(process.execPath, [launcherPath, command], {
      encoding: 'utf8',
      env: {
        ...process.env,
        ...shimEnvironment,
        PATH: `${binDir}${delimiter}${process.env.PATH ?? ''}`,
        WAILS_CAPTURE_FILE: captureFile,
        EXTRA_TAGS: 'ambient-build',
        LTHN_DEV: 'ambient-dev',
        LTHN_WAILS_WS_LISTEN: '127.0.0.1:7777',
        LTHN_WAILS_WS_URL: 'ws://localhost:7777/wails/ws',
      },
    });

    assert.equal(result.status, 0, result.stderr);
    const captured = JSON.parse(await readFile(captureFile, 'utf8'));
    assert.deepEqual(captured.args, ['task', command]);
    assert.deepEqual(captured.managed, expectedManaged);
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
