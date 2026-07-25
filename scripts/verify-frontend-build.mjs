import { access, readdir, readFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const REQUIRED_FAMILIES = ['Geist', 'Geist Mono', 'Instrument Serif', 'Font Awesome 7 Free'];

export async function verifyFrontendBuild(distDir) {
  const cssFiles = (await readdir(distDir)).filter((name) => name.endsWith('.css'));
  const css = (
    await Promise.all(cssFiles.map((name) => readFile(join(distDir, name), 'utf8')))
  ).join('\n');
  const indexHTML = await readFile(join(distDir, 'index.html'), 'utf8');
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
  const stylesheetLinks = [...indexHTML.matchAll(/<link\b[^>]*>/gi)]
    .map((match) => match[0])
    .filter((tag) => /\brel\s*=\s*["']stylesheet["']/i.test(tag));
  if (stylesheetLinks.length === 0) {
    throw new Error('missing active stylesheet link');
  }
  for (const link of stylesheetLinks) {
    const usesInlineActivation = /\bonload\s*=/i.test(link);
    const isPrintOnly = /\bmedia\s*=\s*["']print["']/i.test(link);
    if (usesInlineActivation || isPrintOnly) {
      throw new Error(`stylesheet activation depends on inline script: ${link}`);
    }
  }
  return { requiredFamilies: REQUIRED_FAMILIES, references, missingAssets, stylesheetLinks };
}

const isMain = process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  const requestedPath = process.argv[2];
  if (!requestedPath) throw new Error('usage: verify-frontend-build.mjs DIST_DIR');
  const report = await verifyFrontendBuild(resolve(requestedPath));
  console.log(
    `frontend font build: ${report.requiredFamilies.length} families, ` +
      `${report.references.length} references, ` +
      `${report.stylesheetLinks.length} stylesheet links, PASS`,
  );
}
