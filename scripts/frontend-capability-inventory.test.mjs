import assert from 'node:assert/strict';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  collectCapabilityEvidence,
  renderCapabilityMatrix,
} from './frontend-capability-inventory.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

test('inventories every base app and routed surface without calling it live', async () => {
  const report = await collectCapabilityEvidence(repoRoot);

  assert.equal(report.baseApps.length, 23);
  assert.equal(report.surfaceApps.length, 43);
  assert.equal(report.surfaceApps.filter(({ contracts }) => contracts.length > 0).length, 35);
  assert.equal(new Set(report.surfaceApps.map(({ route }) => route)).size, 43);
  assert.equal(report.entries.some(({ sourceState }) => sourceState === 'live'), false);
});

test('renders evidence and limitations instead of mock-or-live guesses', async () => {
  const markdown = renderCapabilityMatrix(await collectCapabilityEvidence(repoRoot));

  assert.match(markdown, /^# Frontend Capability Matrix/m);
  assert.match(markdown, /Source state is not runtime maturity/);
  assert.match(markdown, /dappco\.re\/lthn\/desktop\/pkg\/tasks\.Service\.List/);
  assert.match(markdown, /\/v1\/ml-lab\/duckdb/);
  assert.doesNotMatch(markdown, /\| Live \|/);
});
