import { access, readdir, readFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const REQUIRED_FAMILIES = ['Geist', 'Geist Mono', 'Instrument Serif', 'Font Awesome 7 Free'];

function parseLinkAttributes(tag) {
  const attributes = new Map();
  const source = tag.replace(/^<link(?=[\s/>])/i, '').replace(/>$/, '');
  const attributePattern =
    /(?:^|\s+)([^\s"'<>\/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g;

  for (const match of source.matchAll(attributePattern)) {
    const name = match[1].toLowerCase();
    const value = match[2] ?? match[3] ?? match[4] ?? '';
    attributes.set(name, value);
  }
  return attributes;
}

function hasAttributeToken(value, expected) {
  return value
    .trim()
    .split(/\s+/)
    .some((token) => token.toLowerCase() === expected);
}

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
  const uncommentedIndex = indexHTML.replace(/<!--[\s\S]*?-->/g, '');
  const stylesheetEntries = [...uncommentedIndex.matchAll(/<link(?=[\s/>])[^>]*>/gi)]
    .map((match) => {
      const tag = match[0];
      return { tag, attributes: parseLinkAttributes(tag) };
    })
    .filter(({ attributes }) => {
      const rel = attributes.get('rel');
      return rel !== undefined && hasAttributeToken(rel, 'stylesheet');
    });
  const stylesheetLinks = stylesheetEntries.map(({ tag }) => tag);
  if (stylesheetLinks.length === 0) {
    throw new Error('missing active stylesheet link');
  }
  for (const { tag: link, attributes } of stylesheetEntries) {
    const usesInlineActivation = attributes.has('onload');
    const isPrintOnly = attributes.get('media')?.trim().toLowerCase() === 'print';
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
    `frontend build: ${report.requiredFamilies.length} families, ` +
      `${report.references.length} references, ` +
      `stylesheet links: ${report.stylesheetLinks.length}, PASS`,
  );
}
