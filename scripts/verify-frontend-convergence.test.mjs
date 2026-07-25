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
