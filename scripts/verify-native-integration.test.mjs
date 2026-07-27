// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const text = async (path) => await readFile(new URL(`../${path}`, import.meta.url), 'utf8');

test('build metadata declares one Lethean desktop identity and native launch contract', async () => {
  const [
    config,
    mac,
    macDev,
    linux,
    linuxGenerated,
    nfpm,
    mime,
    nsis,
    msix,
    msixTemplate,
  ] = await Promise.all([
    text('build/config.yml'),
    text('build/darwin/Info.plist'),
    text('build/darwin/Info.dev.plist'),
    text('build/linux/lthn.desktop'),
    text('build/linux/desktop'),
    text('build/linux/nfpm/nfpm.yaml'),
    text('build/linux/application-x-lethean.xml'),
    text('build/windows/nsis/project.nsi'),
    text('build/windows/msix/app_manifest.xml'),
    text('build/windows/msix/template.xml'),
  ]);

  assert.match(config, /productIdentifier:\s*"ai\.lthn\.desktop"/);
  assert.match(config, /ext:\s*lthn\b/);
  assert.match(config, /mimeType:\s*application\/x-lethean\b/);
  assert.match(config, /scheme:\s*lthn\b/);

  for (const plist of [mac, macDev]) {
    assert.match(plist, /<key>CFBundleExecutable<\/key>\s*<string>lthn<\/string>/);
    assert.match(plist, /<string>ai\.lthn\.desktop<\/string>/);
    assert.match(plist, /<string>application\/x-lethean<\/string>/);
    assert.match(plist, /<string>lthn<\/string>/);
  }

  for (const desktop of [linux, linuxGenerated]) {
    assert.match(desktop, /^Name=Lethean Desktop$/m);
    assert.match(desktop, /^Exec=\/usr\/local\/bin\/lthn %U$/m);
    assert.match(
      desktop,
      /^MimeType=x-scheme-handler\/lthn;application\/x-lethean;$/m,
    );
  }
  assert.match(mime, /type="application\/x-lethean"/);
  assert.match(mime, /pattern="\*\.lthn"/);
  assert.match(nfpm, /application-x-lethean\.xml/);
  assert.match(nfpm, /\/usr\/share\/mime\/packages\/application-x-lethean\.xml/);

  assert.match(nsis, /wails\.associateFiles/);
  assert.match(nsis, /wails\.associateCustomProtocols/);

  assert.match(msix, /Name="ai\.lthn\.desktop"/);
  assert.match(msix, /Publisher="CN=Lethean"/);
  assert.match(msix, /Executable="lthn\.exe"/);
  assert.match(msix, /Category="windows\.protocol"/);
  assert.match(msix, /<uap:Protocol Name="lthn">/);
  assert.match(msix, /Category="windows\.fileTypeAssociation"/);
  assert.match(msix, /<uap:FileType>\.lthn<\/uap:FileType>/);

  assert.doesNotMatch(msix, /scaffold|My Company/i);
  assert.doesNotMatch(msixTemplate, /scaffold|My Company/i);
  assert.match(msixTemplate, /ExecutableName="lthn\.exe"/);
});
