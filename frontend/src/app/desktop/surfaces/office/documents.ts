import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-office-documents-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OfficeDocumentsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'office-documents',
    title: $localize`:Office documents title@@surface.office.documents.title:Documents`,
    subtitle: $localize`:Office documents subtitle@@surface.office.documents.subtitle:6 recent · ~/.lthn/docs/`,
    icon: 'file-lines',
    actions: [{ id: 'new', label: 'New document', icon: 'plus', kind: 'add' }],
    filters: [
      { id: 'draft', label: 'Draft' },
      { id: 'ready', label: 'Ready' },
      { id: 'final', label: 'Final' },
      { id: 'live', label: 'Live' },
    ],
    rows: [
      {
        id: 'release-notes',
        title: 'v0.2 release notes',
        meta: 'you · now',
        status: 'draft',
        value: '4.2 KB',
      },
      {
        id: 'manifesto',
        title: 'Sovereign-compute manifesto',
        meta: 'you · yesterday',
        status: 'ready',
        value: '12 KB',
      },
      {
        id: 'board-pack',
        title: 'Q2 board pack · numbers',
        meta: 'Mei · 3 d ago',
        status: 'final',
        value: '248 KB',
      },
      {
        id: 'hiring',
        title: 'Hiring · senior Go engineer',
        meta: 'you · 1 w ago',
        status: 'live',
        value: '6.4 KB',
      },
      {
        id: 'sow',
        title: 'SOW · Heritage Law LLP v2',
        meta: 'you · 4 d ago',
        status: 'draft',
        value: '38 KB',
      },
      {
        id: 'dns-runbook',
        title: 'Runbook · DNS rotation',
        meta: 'Tobi · 2 w ago',
        status: 'final',
        value: '8.2 KB',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/office/documents.Service.List',
    pollMs: 30_000,
    bridgeArgs: [{}],
    liveKeys: ['documents'],
    footer: 'local markdown · auto-syncs across machines · ⌘N to write · pkg/office/documents',
  };
}
