import { access, readdir, readFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const REQUIRED_FAMILIES = ['Geist', 'Geist Mono', 'Instrument Serif', 'Font Awesome 7 Free'];

export async function verifyFrontendBuild(distDir) {
  const cssFiles = (await readdir(distDir)).filter((name) => name.endsWith('.css'));
  const css = (
    await Promise.all(cssFiles.map((name) => readFile(join(distDir, name), 'utf8')))
  ).join('\n');
  const references = [...css.matchAll(/url\(["']?(\.\/media\/[^)"']+\.(?:woff2?|ttf|otf))/g)].map(
    (match) => match[1],
  );
  const missingAssets = [];

  for (const reference of new Set(references)) {
    const path = join(distDir, reference.replace(/^\.\//, ''));
    try {
      await access(path);
    } catch {
      missingAssets.push(reference);
    }
  }
  if (missingAssets.length > 0) {
    throw new Error(`missing font asset: ${missingAssets.join(', ')}`);
  }
  for (const family of REQUIRED_FAMILIES) {
    const escaped = family.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const declaration = new RegExp(`font-family:\\s*["']?${escaped}["']?(?:;|})`);
    if (!declaration.test(css)) {
      throw new Error(`missing required font family: ${family}`);
    }
  }
  return { requiredFamilies: REQUIRED_FAMILIES, references, missingAssets };
}

const isMain = process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  const requestedPath = process.argv[2];
  if (!requestedPath) throw new Error('usage: verify-frontend-build.mjs DIST_DIR');
  const report = await verifyFrontendBuild(resolve(requestedPath));
  console.log(
    `frontend font build: ${report.requiredFamilies.length} families, ` +
      `${report.references.length} references, PASS`,
  );
}
