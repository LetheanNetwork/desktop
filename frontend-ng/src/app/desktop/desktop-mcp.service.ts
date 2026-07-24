import { Service, declareExperimentalWebMcpTool, inject } from '@angular/core';
import { APPS, ViewMode, Win } from './desktop.data';
import { WindowManagerService } from './window-manager.service';

const APP_IDS = Object.keys(APPS);
const VIEW_MODES = ['desktop', 'shell', 'device'] as const;

const EMPTY_SCHEMA = {
  type: 'object',
  properties: {},
  additionalProperties: false,
} as const;

const APP_SCHEMA = {
  type: 'object',
  properties: {
    app_id: {
      type: 'string',
      enum: APP_IDS,
      description: 'Application id from the desktop app registry.',
    },
  },
  required: ['app_id'],
  additionalProperties: false,
} as const;

const WINDOW_SCHEMA = {
  type: 'object',
  properties: {
    window_id: {
      type: 'string',
      minLength: 1,
      description: 'Id returned by desktop_list_windows.',
    },
  },
  required: ['window_id'],
  additionalProperties: false,
} as const;

const VIEW_SCHEMA = {
  type: 'object',
  properties: {
    view: {
      type: 'string',
      enum: VIEW_MODES,
      description: 'Desktop presentation mode.',
    },
  },
  required: ['view'],
  additionalProperties: false,
} as const;

const toolText = (value: unknown) => ({
  content: [{ type: 'text', text: JSON.stringify(value) }],
});

/**
 * Provider-neutral browser tools for operating the desktop shell.
 *
 * Angular registers these against document.modelContext. The pre-bootstrap
 * Wails adapter mirrors that context to window.__lthnWebMcp so any MCP client
 * with access to the existing webview bridge can list and call the tools.
 */
@Service()
export class DesktopMcpService {
  private readonly windows = inject(WindowManagerService);

  readonly ready = this.registerTools();

  private async registerTools(): Promise<void> {
    await Promise.all([
      declareExperimentalWebMcpTool({
        name: 'desktop_launch_app',
        description: 'Launches a registered desktop application, or focuses its existing window.',
        inputSchema: APP_SCHEMA,
        execute: ({ app_id }) => {
          const app = this.requireApp(app_id);
          this.windows.launch(app.id);
          return toolText({ ok: true, app });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_focus_window',
        description: 'Focuses an open desktop window and restores it if it is minimised.',
        inputSchema: WINDOW_SCHEMA,
        execute: ({ window_id }) => {
          const win = this.requireWindow(window_id);
          this.windows.focus(win.id);
          return toolText({ ok: true, window_id: win.id });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_close_window',
        description: 'Closes an open desktop window.',
        inputSchema: WINDOW_SCHEMA,
        execute: ({ window_id }) => {
          const win = this.requireWindow(window_id);
          this.windows.close(win.id);
          return toolText({ ok: true, window_id: win.id });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_minimise_window',
        description: 'Minimises an open desktop window.',
        inputSchema: WINDOW_SCHEMA,
        execute: ({ window_id }) => {
          const win = this.requireWindow(window_id);
          if (!win.min) this.windows.minimise(win.id);
          return toolText({
            ok: true,
            window_id: win.id,
            changed: !win.min,
          });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_maximise_window',
        description:
          'Maximises an open desktop window without toggling an already maximised window.',
        inputSchema: WINDOW_SCHEMA,
        execute: ({ window_id }) => {
          const win = this.requireWindow(window_id);
          if (!win.max) this.windows.maximise(win.id);
          return toolText({
            ok: true,
            window_id: win.id,
            changed: !win.max,
          });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_list_windows',
        description: 'Lists open desktop windows with app identity, focus, view, and window state.',
        inputSchema: EMPTY_SCHEMA,
        execute: () =>
          toolText({
            view: this.windows.view(),
            focused_window_id: this.windows.focusId(),
            windows: this.windows.openWins().map((win) => this.describeWindow(win)),
          }),
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_read_state',
        description: 'Reads the current desktop view mode, device mode, and focused application.',
        inputSchema: EMPTY_SCHEMA,
        execute: () => {
          const focused =
            this.windows.openWins().find((win) => win.id === this.windows.focusId()) ?? null;
          return toolText({
            view: this.windows.view(),
            device: this.windows.device(),
            windowed: this.windows.windowed(),
            focused: focused ? this.describeWindow(focused) : null,
          });
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'desktop_switch_view',
        description: 'Switches the desktop presentation between desktop, shell, and device modes.',
        inputSchema: VIEW_SCHEMA,
        execute: ({ view }) => {
          this.requireView(view);
          this.windows.setView(view);
          return toolText({ ok: true, view });
        },
      }),
    ]);
  }

  private requireApp(appId: string) {
    const app = this.windows.app(appId);
    if (!app || !Object.hasOwn(APPS, appId)) {
      throw new Error(`Unknown app id "${appId}". Expected one of: ${APP_IDS.join(', ')}.`);
    }
    return app;
  }

  private requireWindow(windowId: string): Win {
    const win = this.windows.openWins().find((candidate) => candidate.id === windowId);
    if (!win) throw new Error(`Unknown open window id "${windowId}".`);
    return win;
  }

  private requireView(view: string): asserts view is ViewMode {
    if (!(VIEW_MODES as readonly string[]).includes(view)) {
      throw new Error(`Unknown view "${view}". Expected one of: ${VIEW_MODES.join(', ')}.`);
    }
  }

  private describeWindow(win: Win) {
    const app = this.windows.app(win.app);
    return {
      id: win.id,
      app_id: win.app,
      title: app?.title ?? win.app,
      focused: win.id === this.windows.focusId(),
      minimised: win.min,
      maximised: win.max,
      subview: win.sub,
      system_tab: win.systab ?? '',
    };
  }
}
