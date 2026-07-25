import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runInNewContext } from 'node:vm';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { verifyFrontendBuild } from './verify-frontend-build.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const activeIndex =
  '<!doctype html><html><head>' +
  '<link rel="stylesheet" href="styles.css">' +
  '</head><body></body></html>';

async function writeCompleteFontFixture(root) {
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
}

test('development transport bootstrap publishes the loopback default only', async () => {
  const source = await readFile(`${repoRoot}/frontend-ng/public/wails/transport.js`, 'utf8');
  const sandbox = { globalThis: {} };
  runInNewContext(source, sandbox);
  assert.deepEqual(JSON.parse(JSON.stringify(sandbox.globalThis.__LTHN_CONNECTION__)), {
    webSocketUrl: 'ws://localhost:9099/wails/ws',
  });
  assert.equal(Object.isFrozen(sandbox.globalThis.__LTHN_CONNECTION__), true);
});

test('font verification rejects a referenced asset that is absent', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(join(root, 'index.html'), activeIndex);
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
  await writeFile(join(root, 'index.html'), activeIndex);
  await writeCompleteFontFixture(root);

  const report = await verifyFrontendBuild(root);
  assert.deepEqual(report.requiredFamilies, [
    'Geist',
    'Geist Mono',
    'Instrument Serif',
    'Font Awesome 7 Free',
  ]);
  assert.equal(report.missingAssets.length, 0);
  assert.deepEqual(report.stylesheetLinks, ['<link rel="stylesheet" href="styles.css">']);
});

test('stylesheet verification rejects inline critical activation', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(
    join(root, 'index.html'),
    '<!doctype html><html><head>' +
      '<link rel="stylesheet" href="styles.css" onload="this.media=\'all\'">' +
      '</head><body></body></html>',
  );
  await writeCompleteFontFixture(root);

  await assert.rejects(
    verifyFrontendBuild(root),
    /stylesheet activation depends on inline script/,
  );
});

test('stylesheet verification rejects print-only links', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(
    join(root, 'index.html'),
    '<!doctype html><html><head>' +
      '<link rel="stylesheet" href="styles.css" media="print">' +
      '</head><body></body></html>',
  );
  await writeCompleteFontFixture(root);

  await assert.rejects(
    verifyFrontendBuild(root),
    /stylesheet activation depends on inline script/,
  );
});

test('stylesheet verification rejects a missing stylesheet link', async () => {
  const root = await mkdtemp(join(tmpdir(), 'lthn-fonts-'));
  await writeFile(
    join(root, 'index.html'),
    '<!doctype html><html><head>' +
      '<link rel="icon" href="favicon.ico">' +
      '</head><body></body></html>',
  );
  await writeCompleteFontFixture(root);

  await assert.rejects(verifyFrontendBuild(root), /missing active stylesheet link/);
});
