import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { chmod, mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { runInNewContext } from 'node:vm';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { verifyFrontendBuild } from './verify-frontend-build.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const execFileAsync = promisify(execFile);

test('development transport bootstrap publishes the loopback default only', async () => {
  const source = await readFile(`${repoRoot}/frontend-ng/public/wails/transport.js`, 'utf8');
  const sandbox = { globalThis: {} };
  runInNewContext(source, sandbox);
  assert.deepEqual(JSON.parse(JSON.stringify(sandbox.globalThis.__LTHN_CONNECTION__)), {
    webSocketUrl: 'ws://localhost:9099/wails/ws',
  });
  assert.equal(Object.isFrozen(sandbox.globalThis.__LTHN_CONNECTION__), true);
});

test('Wails development commands apply environment through an executable', async () => {
  const config = await readFile(`${repoRoot}/build/config.yml`, 'utf8');
  const commands = [...config.matchAll(/^\s*-\s+cmd:\s+(.+)$/gm)]
    .map((match) => match[1])
    .filter((command) => /wails3 task (?:build|run)$/.test(command));
  assert.equal(commands.length, 2);

  const binDir = await mkdtemp(join(tmpdir(), 'lthn-wails-dev-'));
  const fakeWails = join(binDir, 'wails3');
  await writeFile(
    fakeWails,
    '#!/bin/sh\nprintf "%s|%s|%s\\n" "${EXTRA_TAGS-}" "${LTHN_DEV-}" "$*"\n',
  );
  await chmod(fakeWails, 0o755);

  const outputs = [];
  for (const command of commands) {
    const [file, ...args] = command.split(/\s+/);
    const { stdout } = await execFileAsync(file, args, {
      env: { ...process.env, PATH: `${binDir}:${process.env.PATH}` },
    });
    outputs.push(stdout.trim());
  }

  assert.deepEqual(outputs, ['mcp||task build', '|1|task run']);
});

test('font verification rejects a referenced asset that is absent', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(
    join(root, 'styles.css'),
    '@font-face{font-family:"Geist";src:url("./media/geist.woff2")}\n' +
      '@font-face{font-family:"Geist Mono";src:url("./media/mono.woff2")}\n' +
      '@font-face{font-family:"Instrument Serif";src:url("./media/serif.woff2")}\n' +
      '@font-face{font-family:"Font Awesome 7 Free";src:url("./media/icons.woff2")}',
  );
  await assert.rejects(verifyFrontendBuild(root), /missing font asset/);
});

test('font verification accepts complete required families', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await mkdir(join(root, 'media'));
  for (const name of ['geist', 'mono', 'serif', 'icons']) {
    await writeFile(join(root, 'media', `${name}.woff2`), name);
  }
  await writeFile(
    join(root, 'styles.css'),
    [
      ['Geist', 'geist'],
      ['Geist Mono', 'mono'],
      ['Instrument Serif', 'serif'],
      ['Font Awesome 7 Free', 'icons'],
    ]
      .map(
        ([family, file]) => `@font-face{font-family:"${family}";src:url("./media/${file}.woff2")}`,
      )
      .join('\n'),
  );

  const report = await verifyFrontendBuild(root);
  assert.deepEqual(report.requiredFamilies, [
    'Geist',
    'Geist Mono',
    'Instrument Serif',
    'Font Awesome 7 Free',
  ]);
  assert.equal(report.missingAssets.length, 0);
});
