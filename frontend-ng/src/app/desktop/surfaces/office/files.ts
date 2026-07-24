import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-office-files-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OfficeFilesSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'office-files',
    title: $localize`:Office files title@@surface.office.files.title:Files`,
    subtitle: $localize`:Office files subtitle@@surface.office.files.subtitle:~/Documents · 5 recent`,
    icon: 'folder-open',
    metrics: [
      { label: 'Disk free', value: '312 GB', hint: '1 TB total', tone: 'brand' },
      { label: 'Indexed', value: '458', hint: 'local items' },
      { label: 'Disk used', value: '68%' },
    ],
    filters: [
      { id: 'code', label: 'Code' },
      { id: 'documents', label: 'Documents' },
      { id: 'models', label: 'lthn / models' },
      { id: 'recordings', label: 'Recordings' },
      { id: 'screenshots', label: 'Screenshots' },
    ],
    rows: [
      {
        id: 'sow',
        title: 'sow-heritage-law-v2.md',
        meta: '~/Documents/sales/',
        value: '38 KB',
        secondary: '14:42',
        tags: ['documents'],
      },
      {
        id: 'release',
        title: 'v0.2-release-notes.md',
        meta: '~/Code/lthn/docs/',
        value: '4.2 KB',
        secondary: '14:18',
        tags: ['code'],
      },
      {
        id: 'model',
        title: 'gemma-4-e2b-q4_k_m.gguf',
        meta: '~/.lthn/models/',
        value: '2.1 GB',
        secondary: 'yesterday',
        tags: ['models'],
      },
      {
        id: 'calliope',
        title: 'calliope-call-notes.md',
        meta: '~/Documents/investors/',
        value: '6.8 KB',
        secondary: 'yesterday',
        tags: ['documents'],
      },
      {
        id: 'icon',
        title: 'lthn-icon-helmet@2x.png',
        meta: '~/Code/lthn/desktop/assets/',
        value: '82 KB',
        secondary: '3 d',
        tags: ['code'],
      },
    ],
    searchPlaceholder: 'Find indexed files',
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/office/files.Service.ListRecent',
    pollMs: 60_000,
    bridgeArgs: [{}],
    liveKeys: ['recents'],
    footer: 'local filesystem · indexed by lthn · ⌘O to open · ⌘⇧P to find',
  };
}
