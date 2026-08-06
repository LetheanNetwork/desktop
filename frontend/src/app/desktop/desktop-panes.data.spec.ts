import { PANE_REGISTRY } from './apps/app-view';
import { APPS } from './desktop-catalogue.data';
import { APP_PANES, paneFor, panesFor } from './desktop-panes.data';

describe('desktop pane catalogue', () => {
  it('declares the consolidated pane tree', () => {
    const tree = [...new Set(APP_PANES.map(({ app }) => app))].map((app) => [
      app,
      panesFor(app).map(({ path }) => path),
    ]);

    expect(tree).toEqual([
      ['activity', ['streams', 'observe']],
      ['operations', ['incidents', 'runbooks', 'status']],
      ['marketplace', ['store', 'plugin-view', 'opencode']],
      [
        'ide',
        [
          'control-panel',
          'explorer',
          'search',
          'git',
          'terminal',
          'build',
          'processes',
          'containers',
          'forge',
          'devops',
        ],
      ],
      ['scm', ['repositories', 'pull-requests', 'deploys']],
      ['databases', ['duckdb', 'influxdb']],
      [
        'project-manager',
        ['issues', 'board', 'today', 'backlog', 'sprints', 'roadmap', 'calendar', 'retros'],
      ],
      ['crm', ['contacts', 'deals', 'pipeline', 'forecast']],
      ['marketing', ['campaigns', 'content', 'social', 'audience', 'analytics']],
      [
        'agents',
        [
          'activity',
          'code',
          'connect',
          'dispatch',
          'flows',
          'scan',
          'tasks',
          'terminal',
          'workspaces',
        ],
      ],
      ['ml-lab', ['workbench', 'models', 'lora']],
      ['files', ['home', 'workspace']],
    ]);
    expect(APP_PANES).toHaveLength(54);
  });

  it('binds every pane to a registered application and exactly one component', () => {
    for (const pane of APP_PANES) {
      expect(APPS[pane.app], pane.app).toBeDefined();
      const content = [pane.surface, pane.view, pane.dev].filter((value) => value !== undefined);
      expect(content, `${pane.app}/${pane.path}`).toHaveLength(1);
      if (pane.dev) continue;
      expect(PANE_REGISTRY[pane.surface ?? pane.view!], `${pane.app}/${pane.path}`).toBeTypeOf(
        'function',
      );
    }
    const paths = APP_PANES.map(({ app, path }) => `${app}/${path}`);
    expect(new Set(paths).size).toBe(paths.length);
  });

  it('falls back to the first pane for an unknown or state-carrying sub', () => {
    expect(paneFor('ide', 'git')?.path).toBe('git');
    // A retired pane path degrades to the application's default pane.
    expect(paneFor('ide', 'repos')?.path).toBe('control-panel');
    expect(paneFor('agents', '')?.path).toBe('activity');
    // Files keeps its location token in `sub`; that is still the Local pane.
    expect(paneFor('files', 'workspace')?.path).toBe('workspace');
    expect(paneFor('files', 'documents::Reports')?.path).toBe('home');
    // An application without panes has none to fall back to.
    expect(paneFor('chat', '')).toBeUndefined();
    expect(panesFor('chat')).toEqual([]);
  });
});
