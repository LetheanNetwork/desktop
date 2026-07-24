import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-workspaces-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsWorkspacesSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-workspaces',
    title: $localize`:Agents workspaces title@@surface.agents.workspaces.title:Workspaces`,
    subtitle: $localize`:Agents workspaces subtitle@@surface.agents.workspaces.subtitle:Prepare a workspace and inspect the context before dispatch`,
    icon: 'folder-tree',
    kind: 'editor',
    editorLabel: 'Workspace preparation request · JSON',
    editorText: `{
  "repo": "desktop",
  "issue": 0,
  "agent": "claude-code"
}`,
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.Prep',
    bridgeInput: 'json',
    actions: [
      { id: 'prepare', label: 'Prepare workspace', icon: 'wand-magic-sparkles', kind: 'run' },
    ],
    metrics: [
      { label: 'Memories recalled', value: '12', tone: 'brand' },
      { label: 'Consumers found', value: '8' },
      { label: 'Prompt version', value: 'v3' },
    ],
    sections: [
      {
        title: 'Repository',
        body: 'desktop · lane/surface-to-hash · resume an existing branch when present',
        tone: 'brand',
      },
      {
        title: 'Assembled prompt',
        items: [
          'Issue body and acceptance gates',
          'Relevant codebase memories',
          'Downstream-consumer impact',
          'Wiki and recent history',
        ],
      },
      {
        title: 'Workspace',
        body: '~/Lethean/agent/cladius/Code/lthn/desktop',
      },
    ],
    footer: 'clone/resume + assemble prompt → agentic_prep_workspace · data via Agents.Prep()',
  };
}
