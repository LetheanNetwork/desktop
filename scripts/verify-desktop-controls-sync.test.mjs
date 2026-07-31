// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const read = async (path) => await readFile(resolve(root, path), 'utf8');

test('desktop controls publish one typed revision after durable appconfig commits', async () => {
  const [service, appconfigEvents, desktopEvents] = await Promise.all([
    read('go/pkg/appconfig/service.go'),
    read('go/pkg/appconfig/events.go'),
    read('go/pkg/desktop/appconfig_events.go'),
  ]);

  assert.match(appconfigEvents, /type Event struct/);
  assert.match(appconfigEvents, /Revision string\s+`json:"revision"`/);
  assert.match(appconfigEvents, /Keys\s+\[\]string\s+`json:"keys"`/);
  assert.match(appconfigEvents, /At\s+string\s+`json:"at"`/);
  assert.doesNotMatch(appconfigEvents, /Path|Command|Environment|Token|Credential/);
  assert.match(desktopEvents, /lthn:desktop-controls:changed/);

  const commit = service.indexOf('cfg.Commit()');
  const publish = service.indexOf('s.fireEvent(keys)');
  assert.ok(commit >= 0 && publish > commit, 'appconfig must publish only after Commit succeeds');
});

test('Angular selects exactly one connected or offline controls provider', async () => {
  const [bridge, offlineStore, effects, preferences] = await Promise.all([
    read('frontend/src/app/desktop/desktop-controls-bridge.service.ts'),
    read('frontend/src/app/desktop/desktop-controls-offline.store.ts'),
    read('frontend/src/app/store/desktop-controls.effects.ts'),
    read('frontend/src/app/desktop/preferences.service.ts'),
  ]);

  assert.match(bridge, /changes\(\): Observable<DesktopControlsChangeNotice>/);
  assert.match(bridge, /DESKTOP_CONTROLS_CHANGED_EVENT = 'lthn:desktop-controls:changed'/);
  assert.match(offlineStore, /STORAGE_KEY = 'lthn\.desktop-controls\.v1'/);
  assert.match(offlineStore, /version:\s*1/);
  assert.match(effects, /this\.bridge\.changes\(\)/);
  assert.match(effects, /selectHasDirtyDesktopControls/);
  assert.match(effects, /externalChangePending/);
  assert.doesNotMatch(preferences, /DESKTOP_STORAGE|localStorage|lthn\.prefs/);

  const offlineChanges = bridge.indexOf(
    'if (this.connection.offline()) return this.offlineStore.changes();',
  );
  const wailsRegistration = bridge.indexOf('this.events.on(DESKTOP_CONTROLS_CHANGED_EVENT');
  assert.ok(
    offlineChanges >= 0 && wailsRegistration > offlineChanges,
    'the offline branch must return before Wails event registration',
  );
});

test('project guidance records the reactive controls ownership boundary', async () => {
  const [agents, todo] = await Promise.all([read('AGENTS.md'), read('TODO.md')]);

  assert.match(agents, /desktop-controls-offline\.store\.ts/);
  assert.match(agents, /lthn:desktop-controls:changed/);
  assert.match(agents, /dirty draft/i);
  assert.match(todo, /\[x\].*appconfig.*push/s);
});
