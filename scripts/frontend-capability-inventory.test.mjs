import assert from 'node:assert/strict';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  collectCapabilityEvidence,
  renderCapabilityMatrix,
} from './frontend-capability-inventory.mjs';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

test('inventories every application and pane route without calling it live', async () => {
  const report = await collectCapabilityEvidence(repoRoot);

  assert.equal(report.baseApps.length, 24);
  assert.equal(report.paneApps.length, 56);
  assert.equal(report.paneApps.filter(({ contracts }) => contracts.length > 0).length, 29);
  assert.equal(new Set(report.entries.map(({ route }) => route)).size, 80);
  // Every pane route hangs off the application route it belongs to.
  const applicationRoutes = new Set(report.baseApps.map(({ route }) => route));
  for (const { route } of report.paneApps) {
    assert.ok(
      applicationRoutes.has(route.slice(0, route.lastIndexOf('/'))),
      `pane route ${route} has no application route`,
    );
  }
  // All 45 surface components stay reachable — 43 as panes, Mail and
  // Documents promoted to applications of their own.
  const surfaceComponents = report.entries
    .map(({ component }) => component)
    .filter((component) => component.includes('/desktop/surfaces/'));
  assert.equal(surfaceComponents.length, 45);
  assert.equal(new Set(surfaceComponents).size, 45);
  assert.equal(
    report.entries.some(({ sourceState }) => sourceState === 'live'),
    false,
  );

  for (const id of ['files', 'surface-office-files']) {
    const entry = report.entries.find((candidate) => candidate.id === id);
    assert.equal(entry?.sourceState, 'integrated', id);
    assert.equal(
      entry?.evidence.includes('frontend/src/app/desktop/desktop-files-bridge.service.ts'),
      true,
      id,
    );
  }
});

test('renders evidence and limitations instead of mock-or-live guesses', async () => {
  const markdown = renderCapabilityMatrix(await collectCapabilityEvidence(repoRoot));

  assert.match(markdown, /^# Frontend Capability Matrix/m);
  assert.match(markdown, /Source state is not runtime maturity/);
  assert.match(markdown, /dappco\.re\/lthn\/desktop\/pkg\/tasks\.Service\.List/);
  assert.match(markdown, /\/v1\/completions/);
  assert.doesNotMatch(markdown, /\| Live \|/);
});
