import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-connect-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsConnectSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-connect',
    title: $localize`:Agents connect title@@surface.agents.connect.title:Connect Claude Code`,
    subtitle: $localize`:Agents connect subtitle@@surface.agents.connect.subtitle:Attach your editor to this machine's CoreAgent hub`,
    icon: 'plug',
    actions: [{ id: 'recipe', label: 'Load recipe', icon: 'rotate', kind: 'refresh' }],
    sections: [
      {
        title: '1 · Add the marketplace',
        items: [
          'claude plugin marketplace add http://127.0.0.1:9202/marketplace',
          'claude plugin install lethean-agent',
        ],
        tone: 'brand',
      },
      {
        title: '2 · Export the bearer',
        body: 'The bearer is revealed only on request from pkg/keys tier-0 and is never written into ~/.claude.',
      },
      {
        title: '3 · Connect',
        items: [
          'MCP URL · http://127.0.0.1:9202/mcp',
          'Restart Claude Code after adding the local server entry.',
        ],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.ClaudeConnectRecipe',
    footer: 'bearer from pkg/keys tier-0 · read-only · Lethean never writes your ~/.claude config',
  };
}
