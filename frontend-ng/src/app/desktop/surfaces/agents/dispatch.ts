import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-dispatch-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsDispatchSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-dispatch',
    title: $localize`:Agents dispatch title@@surface.agents.dispatch.title:Dispatch`,
    subtitle: $localize`:Agents dispatch subtitle@@surface.agents.dispatch.subtitle:Launch a CoreAgent run across the fleet`,
    icon: 'paper-plane',
    kind: 'editor',
    editorLabel: 'Dispatch request · JSON',
    editorText: `{
  "repo": "desktop",
  "agent": "claude-code",
  "task": "Implement and verify the selected task",
  "persona": "code/senior-developer"
}`,
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.Dispatch',
    bridgeInput: 'json',
    actions: [{ id: 'dispatch', label: 'Dispatch run', icon: 'play', kind: 'run' }],
    rows: [
      {
        id: 'claude-code',
        title: 'Claude Code',
        meta: 'Anthropic · local harness',
        detail: 'Strong repository navigation and implementation',
        status: 'ready',
        tags: ['agent'],
      },
      {
        id: 'codex',
        title: 'Codex',
        meta: 'OpenAI · local harness',
        detail: 'Long-running code changes and verification',
        status: 'ready',
        tags: ['agent'],
      },
      {
        id: 'senior-developer',
        title: 'Senior developer',
        meta: 'code/senior-developer',
        detail: 'Pragmatic implementation persona',
        status: 'persona',
        tags: ['persona'],
      },
      {
        id: 'package-update',
        title: 'Package update',
        meta: 'maintenance',
        detail: 'Update, compile, test and report dependency drift',
        status: 'template',
        tags: ['template'],
      },
    ],
    filters: [
      { id: 'agent', label: 'Agents' },
      { id: 'persona', label: 'Personas' },
      { id: 'template', label: 'Task templates' },
    ],
    sections: [
      {
        title: 'Repository',
        body: 'desktop · lane/surface-to-hash',
        tone: 'brand',
      },
      {
        title: 'Task',
        body: 'Choose a fleet agent, persona and plan template, then add the run instructions.',
      },
    ],
    searchPlaceholder: 'Find agents, personas or task templates',
    footer:
      'repo + agent + task → agentic_dispatch · agent runs detached · data via Agents.Dispatch()',
  };
}
