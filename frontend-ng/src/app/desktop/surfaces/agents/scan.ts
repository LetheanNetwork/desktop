import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-scan-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsScanSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-scan',
    title: $localize`:Agents scan title@@surface.agents.scan.title:Scan`,
    subtitle: $localize`:Agents scan subtitle@@surface.agents.scan.subtitle:Find open Forge issues suitable for dispatch`,
    icon: 'radar',
    kind: 'editor',
    editorLabel: 'Scan request · JSON',
    editorText: `{
  "org": "LetheanNetwork",
  "labels": ["agentic", "help wanted", "bug"]
}`,
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.Scan',
    bridgeInput: 'json',
    actions: [{ id: 'scan', label: 'Scan organisation', icon: 'satellite-dish', kind: 'run' }],
    filters: [
      { id: 'agentic', label: 'agentic' },
      { id: 'help-wanted', label: 'help wanted' },
      { id: 'bug', label: 'bug' },
    ],
    rows: [
      {
        id: '#482',
        title: 'LoRA training UI · final pass',
        meta: 'LetheanNetwork/desktop',
        status: 'open',
        tags: ['agentic'],
      },
      {
        id: '#184',
        title: 'Document MCP tool registry format',
        meta: 'LetheanNetwork/core-go',
        status: 'open',
        tags: ['help-wanted'],
      },
      {
        id: '#278',
        title: 'Metal kernel fails on low-memory devices',
        meta: 'LetheanNetwork/go-mlx',
        status: 'open',
        tags: ['bug'],
      },
    ],
    searchPlaceholder: 'Filter candidates by repository or title',
    footer: 'open Forge issues by label · dispatch via the Dispatch panel · data via Agents.Scan()',
  };
}
