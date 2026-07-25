import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const read = (path) => readFile(`${repoRoot}/${path}`, 'utf8');

test('keeps the Angular root client-rendered without hydration triggers', async () => {
  const template = await read('frontend-ng/src/app/app.html');
  assert.match(template, /@defer \(on immediate\)/);
  assert.doesNotMatch(template, /hydrate\s+on/);
  assert.match(template, /<router-outlet\s*\/>/);
});

test('all binding generators target frontend-ng and no removed external tree', async () => {
  const files = await Promise.all([
    read('build/Taskfile.yml'),
    read('build/ios/Taskfile.yml'),
    read('build/android/Taskfile.yml'),
  ]);
  const combined = files.join('\n');

  assert.doesNotMatch(combined, /frontend\/bindings/);
  assert.doesNotMatch(combined, /external\/gui/);
  assert.match(combined, /frontend-ng\/bindings/);
});

test('CI and audit use the current module and Angular topology', async () => {
  const workflow = await read('.github/workflows/build.yml');
  const audit = await read('build/audit.sh');

  assert.doesNotMatch(workflow, /submodules:\s*recursive/);
  assert.match(audit, /cd frontend-ng/);
  assert.match(audit, /npm run build/);
  assert.match(audit, /npm run test:ci/);
  assert.doesNotMatch(audit, /bun run|cd frontend(?:\s|$)/);
});

test('binding generation synchronises the root Go workspace on every platform', async () => {
  const common = await read('build/Taskfile.yml');
  const platforms = (
    await Promise.all(
      ['darwin', 'linux', 'windows', 'ios', 'android'].map((name) =>
        read(`build/${name}/Taskfile.yml`),
      ),
    )
  ).join('\n');

  assert.match(common, /\n  go:work:sync:/);
  assert.match(common, /- go work sync/);
  assert.doesNotMatch(common, /go:mod:tidy/);
  assert.doesNotMatch(platforms, /common:go:mod:tidy/);
  assert.equal(platforms.match(/common:go:work:sync/g)?.length, 5);
});

test('mobile platforms expose public binding verification tasks', async () => {
  for (const platform of ['ios', 'android']) {
    const taskfile = await read(`build/${platform}/Taskfile.yml`);
    assert.match(
      taskfile,
      new RegExp(
        `\\n  bindings:\\n(?:.*\\n)*?      - task: generate:${platform}:bindings`,
      ),
    );
  }
});

test('mobile binding generators execute the host Wails CLI under target environments', async () => {
  for (const platform of ['ios', 'android']) {
    const taskfile = await read(`build/${platform}/Taskfile.yml`);
    const generator = taskfile.match(
      new RegExp(
        `\\n  generate:${platform}:bindings:\\n[\\s\\S]*?(?=\\n  [^ \\n][^\\n]*:\\n|\\s*$)`,
      ),
    )?.[0];

    assert.ok(generator);
    assert.match(generator, /&& wails3 generate bindings /);
  }
});

test('the root frontend test task runs convergence contracts', async () => {
  const taskfile = await read('Taskfile.yml');
  const frontendTask = taskfile.match(/\n  test:frontend:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/)?.[0];

  assert.ok(frontendTask);
  assert.match(frontendTask, /npm run test:ci/);
  assert.match(frontendTask, /npm run test:contracts/);
});
