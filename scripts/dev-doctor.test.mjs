import assert from 'node:assert/strict';
import test from 'node:test';
import {
  inspectDevelopmentEnvironment,
  renderDoctorReport,
} from './dev-doctor.mjs';

const commandProbe = async ({ name }) => ({
  available: true,
  detail: `${name} test-version`,
});

const portProbe = async (port) => ({
  available: true,
  detail: `127.0.0.1:${port} is available`,
});

const pathProbe = async () => true;

test('reports a ready development environment from injected probes', async () => {
  const report = await inspectDevelopmentEnvironment({
    cwd: '/workspace/desktop',
    homeDir: '/home/developer',
    environment: {},
    commandProbe,
    portProbe,
    pathProbe,
  });

  assert.equal(report.ok, true);
  assert.equal(report.checks.filter(({ status }) => status === 'error').length, 0);
  assert.equal(report.checks.filter(({ status }) => status === 'warning').length, 0);
  assert.match(renderDoctorReport(report), /Ready for development/);
  assert.match(renderDoctorReport(report), /Angular HMR.*9245/);
});

test('fails when a required development command is unavailable', async () => {
  const report = await inspectDevelopmentEnvironment({
    cwd: '/workspace/desktop',
    homeDir: '/home/developer',
    environment: {},
    commandProbe: async ({ name }) => ({
      available: name !== 'Wails',
      detail: name === 'Wails' ? 'command not found' : `${name} test-version`,
    }),
    portProbe,
    pathProbe,
  });

  assert.equal(report.ok, false);
  assert.deepEqual(
    report.checks
      .filter(({ status }) => status === 'error')
      .map(({ name }) => name),
    ['Wails'],
  );
  assert.match(
    renderDoctorReport(report),
    /Restore the Wails tool declared by go\/go\.mod/,
  );
});

test('probes the module-pinned Wails tool instead of a PATH executable', async () => {
  let wailsProbe;
  await inspectDevelopmentEnvironment({
    cwd: '/workspace/desktop',
    homeDir: '/home/developer',
    environment: {},
    commandProbe: async (command) => {
      if (command.name === 'Wails') wailsProbe = command;
      return { available: true, detail: `${command.name} test-version` };
    },
    portProbe,
    pathProbe,
  });

  assert.equal(wailsProbe.command, 'go');
  assert.deepEqual(wailsProbe.args, ['tool', 'wails3', 'version']);
});

test('warns about occupied development ports without hiding other checks', async () => {
  const report = await inspectDevelopmentEnvironment({
    cwd: '/workspace/desktop',
    homeDir: '/home/developer',
    environment: {},
    commandProbe,
    portProbe: async (port) => ({
      available: port !== 9199,
      detail:
        port === 9199
          ? '127.0.0.1:9199 is already in use'
          : `127.0.0.1:${port} is available`,
    }),
    pathProbe,
  });

  assert.equal(report.ok, true);
  assert.deepEqual(
    report.checks
      .filter(({ status }) => status === 'warning')
      .map(({ name }) => name),
    ['Lethean WebSocket (9199)'],
  );
});

test('uses repository overrides and treats absent crew repositories as optional', async () => {
  const seenPaths = [];
  const report = await inspectDevelopmentEnvironment({
    cwd: '/workspace/desktop',
    homeDir: '/home/developer',
    environment: {
      LTHN_MLX_REPO: '/opt/lethean/go-mlx',
      LTHN_AGENT_REPO: '/opt/lethean/agent',
      LTHN_AI_REPO: '/opt/lethean/go-ai',
    },
    commandProbe,
    portProbe,
    pathProbe: async (path) => {
      seenPaths.push(path);
      return !path.startsWith('/opt/lethean/');
    },
  });

  assert.equal(report.ok, true);
  assert.deepEqual(
    report.checks
      .filter(({ status }) => status === 'warning')
      .map(({ name }) => name),
    ['go-mlx repository', 'agent repository', 'go-ai repository'],
  );
  assert.deepEqual(
    seenPaths.filter((path) => path.startsWith('/opt/lethean/')),
    ['/opt/lethean/go-mlx', '/opt/lethean/agent', '/opt/lethean/go-ai'],
  );
});
