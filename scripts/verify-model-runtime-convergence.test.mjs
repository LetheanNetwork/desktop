// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

const read = async (path) => await readFile(resolve(root, path), 'utf8');

test('the managed LEM service remains fixed, local, model-less, and manual', async () => {
  const source = await read('go/pkg/services/service.go');

  assert.match(source, /ID:\s+"inference"/);
  assert.match(
    source,
    /"serve",\s*"--addr",\s*"127\.0\.0\.1:36911",\s*"--shutdown-timeout",\s*"10s"/s,
  );
  assert.doesNotMatch(source, /exec\.LookPath/);
  assert.doesNotMatch(source, /"--model"|"-m"|--cors/);
});

test('production tasks stage one fixed sibling LEM sidecar per desktop platform', async () => {
  const [rootTask, darwinTask, linuxTask, windowsTask, windowsInstaller] =
    await Promise.all([
      read('Taskfile.yml'),
      read('build/darwin/Taskfile.yml'),
      read('build/linux/Taskfile.yml'),
      read('build/windows/Taskfile.yml'),
      read('build/windows/nsis/project.nsi'),
    ]);

  assert.match(rootTask, /LTHN_LEM_REPO/);
  assert.match(rootTask, /LTHN_LEM_BIN/);
  assert.match(rootTask, /pre-build:\s+[\s\S]*?- build:lem/);
  assert.match(rootTask, /package:\s+[\s\S]*?deps: \[pre-build\]/);
  assert.match(rootTask, /task build:embed/);
  assert.match(rootTask, /task build:native/);
  assert.doesNotMatch(rootTask, /command -v lem|which lem|exec\.LookPath/);
  assert.match(darwinTask, /Contents\/MacOS\/lem/);
  assert.match(linuxTask, /\$APPDIR\/usr\/bin\/lem/);
  assert.match(linuxTask, /dst: \\"\/usr\/local\/bin\/lem\\"/);
  assert.match(windowsTask, /ARG_LEM_BINARY/);
  assert.match(windowsInstaller, /File "\/oname=lem\.exe"/);
});

test('the renderer DTOs contain no execution, transport, credential, or native-path fields', async () => {
  const [goTypes, angularTypes] = await Promise.all([
    read('go/pkg/modelruntime/types.go'),
    read('frontend-ng/src/app/desktop/desktop-model-runtime.models.ts'),
  ]);
  const forbidden =
    /(?:json:"(?:path|model_path|command|arguments|environment|working_directory|endpoint|url|token|secret|credential|key)"|readonly\s+(?:path|modelPath|command|arguments|environment|workingDirectory|endpoint|url|token|secret|credential|key)\??:)/iu;

  assert.doesNotMatch(goTypes, forbidden);
  assert.doesNotMatch(angularTypes, forbidden);
});

test('the WebView exposes only ModelRuntime, not the retired model-path or Lemma bindings', async () => {
  const [desktop, liveData, tray] = await Promise.all([
    read('go/pkg/desktop/desktop.go'),
    read('frontend-ng/src/app/desktop/desktop-live-data.service.ts'),
    read('frontend-ng/src/app/tray-panel/tray-panel.ts'),
  ]);

  assert.doesNotMatch(desktop, /gui\.Bind\(models\.NewWailsService/);
  assert.doesNotMatch(desktop, /gui\.Bind\(lemma\.NewWailsService/);
  assert.doesNotMatch(liveData, /pkg\/models\.WailsService|LocalModelEntry/);
  assert.doesNotMatch(tray, /pkg\/lemma\.WailsService|model_path|basename\(/);
  assert.match(tray, /MODEL_RUNTIME_METHODS\.snapshot/);
});
