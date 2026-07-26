import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-code-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsCodeSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-code',
    title: $localize`:Agents code title@@surface.agents.code.title:Code`,
    subtitle: $localize`:Agents code subtitle@@surface.agents.code.subtitle:Read-only source view for the selected finding`,
    icon: 'code',
    kind: 'editor',
    editorLabel: 'go/pkg/agents/cli.go · line 246',
    editorText: `func (s *Service) Dispatch(req DispatchRequest) core.Result {
    if core.Trim(req.Repo) == "" {
        return core.Fail(core.E("agents.Dispatch", "repo required", nil))
    }
    return s.runJSON("dispatch", req)
}`,
    actions: [{ id: 'copy', label: 'Copy', icon: 'copy', kind: 'copy' }],
    sections: [
      {
        title: 'Finding',
        body: 'Open a task from Backlog to load its repository, file and target line here.',
        tone: 'brand',
      },
      {
        title: 'Safety',
        body: 'The viewer reads through Files.Preview({mountId, path}) and never writes the selected source file.',
      },
    ],
    footer:
      'read-only source view · Files.Preview({mountId, path}) · open a finding from the Backlog',
  };
}
