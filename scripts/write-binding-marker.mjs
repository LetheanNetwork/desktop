import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

const markerPath = process.argv[2];

if (!markerPath) {
  throw new Error('usage: write-binding-marker.mjs MARKER_PATH');
}

const resolvedPath = resolve(markerPath);
await mkdir(dirname(resolvedPath), { recursive: true });
await writeFile(resolvedPath, '');
