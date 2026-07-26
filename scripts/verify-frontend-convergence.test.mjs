import assert from 'node:assert/strict';
import { execFile as execFileCallback } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { delimiter, dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const read = (path) => readFile(`${repoRoot}/${path}`, 'utf8');
const execFile = promisify(execFileCallback);

test('keeps the Angular root client-rendered without hydration triggers', async () => {
  const template = await read('frontend-ng/src/app/app.html');
  assert.match(template, /@defer \(on immediate\)/);
  assert.doesNotMatch(template, /hydrate\s+on/);
  assert.match(template, /<router-outlet\s*\/>/);
});

test('frontend exposes the documented deterministic demo server', async () => {
  const packageJSON = JSON.parse(await read('frontend-ng/package.json'));
  assert.equal(
    packageJSON.scripts.demo,
    'ng serve --host 127.0.0.1 --port 9245 --hmr --poll 1000',
  );
});

test('Wails development installs frontend dependencies once before starting HMR', async () => {
  const config = await read('build/config.yml');
  const common = await read('build/Taskfile.yml');
  const root = await read('Taskfile.yml');
  const installFrontend = common.match(
    /\n  install:frontend:deps:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];
  const devFrontend = common.match(
    /\n  dev:frontend:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];
  const devBuild = root.match(
    /\n  dev:build:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];

  assert.ok(installFrontend);
  assert.ok(devFrontend);
  assert.ok(devBuild);
  assert.equal(config.match(/cmd: wails3 task common:install:frontend:deps/g)?.length, 1);
  assert.match(config, /cmd: wails3 task common:install:frontend:deps\n\s+type: once/);
  assert.match(config, /cmd: wails3 task common:dev:frontend\n\s+type: background/);
  assert.match(installFrontend, /generates:\n\s+- node_modules\/\.package-lock\.json/);
  assert.doesNotMatch(devFrontend, /\n\s+deps:/);
  assert.doesNotMatch(devFrontend, /- task: install:frontend:deps/);
  assert.match(devFrontend, /test -d node_modules/);
  assert.match(devBuild, /go build/);
  assert.match(devBuild, /-gcflags=all=/);
  assert.doesNotMatch(devBuild, /-tags production/);
  assert.doesNotMatch(devBuild, /pre-build|build:frontend|install:frontend:deps/);
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
      new RegExp(`\\n  bindings:\\n(?:.*\\n)*?      - task: generate:${platform}:bindings`),
    );
  }
});

test('binding tasks use the host Wails CLI and restore desktop bindings after mobile generation', async (t) => {
  const harnessRoot = await mkdtemp(join(tmpdir(), 'lthn-bindings-contract-'));
  const canonicalHarnessRoot = await realpath(harnessRoot);
  t.after(() => rm(harnessRoot, { recursive: true, force: true }));

  const taskfiles = [
    'Taskfile.yml',
    'build/Taskfile.yml',
    'build/darwin/Taskfile.yml',
    'build/linux/Taskfile.yml',
    'build/windows/Taskfile.yml',
    'build/ios/Taskfile.yml',
    'build/android/Taskfile.yml',
    'scripts/write-binding-marker.mjs',
  ];
  for (const path of taskfiles) {
    const destination = join(harnessRoot, path);
    await mkdir(dirname(destination), { recursive: true });
    await writeFile(destination, await read(path));
  }

  await mkdir(join(harnessRoot, 'go/pkg/desktop'), { recursive: true });
  await mkdir(join(harnessRoot, 'frontend-ng'), { recursive: true });
  await writeFile(join(harnessRoot, 'go/go.mod'), 'module example.test/desktop\n\ngo 1.26\n');
  await writeFile(join(harnessRoot, 'go/go.sum'), '');
  await writeFile(join(harnessRoot, 'go/pkg/desktop/desktop.go'), 'package desktop\n');

  const binDir = join(harnessRoot, 'test-bin');
  const cacheDir = join(harnessRoot, 'task-cache');
  const recordPath = join(harnessRoot, 'wails-invocations.jsonl');
  await mkdir(binDir, { recursive: true });
  const fakeWailsSource = `
const { appendFileSync, mkdirSync, rmSync, writeFileSync } = require('node:fs');
const { resolve } = require('node:path');

const args = process.argv.slice(2);
const destinationIndex = args.indexOf('-d');
if (destinationIndex < 0 || !args[destinationIndex + 1]) {
  throw new Error('missing bindings destination');
}
const destination = resolve(process.cwd(), args[destinationIndex + 1]);
const record = {
  executable: process.argv[1],
  args,
  cwd: process.cwd(),
  destination,
  clean: args.includes('-clean=true'),
  env: {
    GOOS: process.env.GOOS ?? '',
    CGO_ENABLED: process.env.CGO_ENABLED ?? '',
    GOARCH: process.env.GOARCH ?? '',
    CGO_CFLAGS: process.env.CGO_CFLAGS ?? '',
    CGO_LDFLAGS: process.env.CGO_LDFLAGS ?? '',
  },
};

rmSync(destination, { recursive: true, force: true });
mkdirSync(destination, { recursive: true });
writeFileSync(resolve(destination, 'binding-owner.json'), JSON.stringify(record));
appendFileSync(process.env.LTHN_BINDING_RECORD, JSON.stringify(record) + '\\n');
`;
  let fakeWails;
  if (process.platform === 'win32') {
    const fakeWailsScript = join(binDir, 'wails3.cjs');
    fakeWails = fakeWailsScript;
    await writeFile(fakeWailsScript, fakeWailsSource);
    await writeFile(
      join(binDir, 'wails3.cmd'),
      `@echo off\r\n"${process.execPath}" "${fakeWailsScript}" %*\r\n`,
    );
    await writeFile(join(binDir, 'go.cmd'), '@exit /b 0\r\n');
    await writeFile(join(binDir, 'xcrun.cmd'), '@echo /fake/sdk\r\n');
    await writeFile(
      join(binDir, 'touch.cmd'),
      '@echo binding tasks must not require POSIX touch 1>&2\r\n@exit /b 91\r\n',
    );
  } else {
    fakeWails = join(binDir, 'wails3');
    await writeFile(fakeWails, `#!/usr/bin/env node\n${fakeWailsSource}`);
    await writeFile(join(binDir, 'go'), '#!/bin/sh\nexit 0\n');
    await writeFile(join(binDir, 'xcrun'), '#!/bin/sh\nprintf "%s\\n" /fake/sdk\n');
    await writeFile(
      join(binDir, 'touch'),
      '#!/bin/sh\necho "binding tasks must not require POSIX touch" >&2\nexit 91\n',
    );
    await Promise.all(
      ['wails3', 'go', 'xcrun', 'touch'].map((name) => chmod(join(binDir, name), 0o755)),
    );
  }

  const env = {
    ...process.env,
    PATH: `${binDir}${delimiter}${process.env.PATH ?? ''}`,
    LTHN_BINDING_RECORD: recordPath,
    NO_COLOR: '1',
  };
  delete env.GOOS;
  delete env.GOARCH;
  delete env.CGO_ENABLED;
  delete env.CGO_CFLAGS;
  delete env.CGO_LDFLAGS;

  const runTask = (...args) =>
    execFile(
      'task',
      ['--taskfile', 'Taskfile.yml', '--temp-dir', cacheDir, '--color=false', ...args],
      {
        cwd: harnessRoot,
        env,
        maxBuffer: 10 * 1024 * 1024,
      },
    );

  await runTask('common:generate:bindings');
  await runTask('common:generate:bindings');
  await runTask('common:generate:bindings', 'BUILD_FLAGS=-tags mcp');
  await runTask('common:generate:bindings');
  await runTask('ios:bindings');
  await runTask('ios:bindings');
  await runTask('ios:bindings', 'IOS_PLATFORM=device');
  await runTask('ios:bindings', 'IOS_PLATFORM=device');
  await runTask('ios:bindings', 'IOS_PLATFORM=simulator', 'ARCH=amd64', 'MIN_IOS_VERSION=16.0');
  await runTask('android:bindings');
  await runTask('common:generate:bindings');

  const records = (await readFile(recordPath, 'utf8'))
    .trim()
    .split('\n')
    .map((line) => JSON.parse(line));
  const bindingsDir = join(canonicalHarnessRoot, 'frontend-ng/bindings');
  const desktopArgs = [
    'generate',
    'bindings',
    '-ts',
    '-d',
    '../frontend-ng/bindings',
    '-f',
    '',
    '-clean=true',
    './pkg/desktop/...',
  ];
  const taggedDesktopArgs = [...desktopArgs];
  taggedDesktopArgs[6] = '-tags mcp';
  const iosArgs = [...desktopArgs];
  iosArgs[6] = '-tags ios';
  const androidArgs = [...desktopArgs];
  androidArgs[6] = '-tags android';

  assert.equal(records.length, 8, 'only repeated desktop and identical iOS targets should skip');
  assert.deepEqual(
    records.map(({ args }) => args),
    [
      desktopArgs,
      taggedDesktopArgs,
      desktopArgs,
      iosArgs,
      iosArgs,
      iosArgs,
      androidArgs,
      desktopArgs,
    ],
  );
  for (const record of records) {
    assert.equal(record.executable, fakeWails);
    assert.equal(record.cwd, join(canonicalHarnessRoot, 'go'));
    assert.equal(record.destination, bindingsDir);
    assert.equal(record.clean, true);
  }

  assert.deepEqual(records[0].env, {
    GOOS: '',
    CGO_ENABLED: '',
    GOARCH: '',
    CGO_CFLAGS: '',
    CGO_LDFLAGS: '',
  });
  assert.deepEqual(records[3].env, {
    GOOS: 'ios',
    CGO_ENABLED: '1',
    GOARCH: 'arm64',
    CGO_CFLAGS:
      '-isysroot /fake/sdk -target arm64-apple-ios15.0-simulator -mios-simulator-version-min=15.0',
    CGO_LDFLAGS: '-isysroot /fake/sdk -target arm64-apple-ios15.0-simulator',
  });
  assert.deepEqual(records[4].env, {
    GOOS: 'ios',
    CGO_ENABLED: '1',
    GOARCH: 'arm64',
    CGO_CFLAGS: '-isysroot /fake/sdk -target arm64-apple-ios15.0 -miphoneos-version-min=15.0',
    CGO_LDFLAGS: '-isysroot /fake/sdk -target arm64-apple-ios15.0',
  });
  assert.deepEqual(records[5].env, {
    GOOS: 'ios',
    CGO_ENABLED: '1',
    GOARCH: 'amd64',
    CGO_CFLAGS:
      '-isysroot /fake/sdk -target x86_64-apple-ios16.0-simulator -mios-simulator-version-min=16.0',
    CGO_LDFLAGS: '-isysroot /fake/sdk -target x86_64-apple-ios16.0-simulator',
  });
  assert.deepEqual(records[6].env, {
    GOOS: 'android',
    CGO_ENABLED: '0',
    GOARCH: '',
    CGO_CFLAGS: '',
    CGO_LDFLAGS: '',
  });

  const finalOwner = JSON.parse(await readFile(join(bindingsDir, 'binding-owner.json'), 'utf8'));
  assert.deepEqual(finalOwner, records.at(-1));
  await readFile(join(bindingsDir, '.desktop-bindings'), 'utf8');
  await assert.rejects(readFile(join(bindingsDir, '.ios-bindings'), 'utf8'));
  await assert.rejects(readFile(join(bindingsDir, '.android-bindings'), 'utf8'));
});

test('the root frontend test task runs convergence contracts', async () => {
  const taskfile = await read('Taskfile.yml');
  const frontendTask = taskfile.match(
    /\n  test:frontend:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];

  assert.ok(frontendTask);
  assert.match(frontendTask, /npm run test:ci/);
  assert.match(frontendTask, /npm run test:contracts/);
});

test('frontend verification has one package, Task, and CI entrypoint', async () => {
  const packageJSON = JSON.parse(await read('frontend-ng/package.json'));
  const rootTaskfile = await read('Taskfile.yml');
  const commonTaskfile = await read('build/Taskfile.yml');
  const workflow = await read('.github/workflows/build.yml');

  assert.equal(packageJSON.scripts.verify, 'node ../scripts/frontend-verify.mjs');

  const verifyTask = rootTaskfile.match(
    /\n  verify:frontend:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];
  assert.ok(verifyTask);
  assert.match(verifyTask, /npm run verify/);

  const installTask = commonTaskfile.match(
    /\n  install:frontend:deps:\n[\s\S]*?(?=\n  [^ \n][^\n]*:\n|\s*$)/,
  )?.[0];
  assert.ok(installTask);
  assert.match(installTask, /npm ci/);
  assert.doesNotMatch(installTask, /npm install/);

  const frontendJob = workflow.match(
    /\n  frontend:\n[\s\S]*?(?=\n  [a-z][a-z0-9_-]*:\n|\s*$)/,
  )?.[0];
  assert.ok(frontendJob);
  assert.match(frontendJob, /run:\s*npm run verify/);
  assert.equal(workflow.match(/npm run verify/g)?.length, 1);
  assert.match(workflow, /\n  build:\n(?:.*\n)*?    needs: frontend\n/);
});
