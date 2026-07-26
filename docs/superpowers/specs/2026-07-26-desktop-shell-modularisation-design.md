# Desktop Shell Modularisation Design

## Goal

Reduce the size and duplication of the Angular desktop shell without changing
its UX, visual design, routes, persistence, controls, or action behaviour.

## Compatibility contract

The existing `DesktopComponent` DOM behaviour is the product contract for this
refactor. In particular:

- every existing control remains present with the same label, icon, tooltip,
  event, and current action, including placeholder actions;
- the menu bar, taskbar, dock, session menu, windows, routes, and device/shell
  modes keep their current behaviour;
- NgRx and `WindowManagerService` remain the owners of window state;
- `DesktopComponent` remains the coordinator during the extraction;
- no Wails/CoreGO capability, persistence format, route, or mock/live data
  behaviour changes in this tranche.

## First tranche

Extract two presentation seams:

1. `ShellMenuBar` renders the Hoplite menu, active app, routed categories,
   language/network/battery/clock tray controls, and emits typed interaction
   requests. It owns no menu or tray state.
2. `ShellTaskbarDock` renders the session chip, task buttons, running-process
   dock, window groups, separator, and Trash control. It owns no window,
   session, group, or drag state.

Both components use required signal inputs, function-based outputs, `OnPush`,
and `ViewEncapsulation.None`. Their hosts use `display: contents` so the new
component boundary does not alter absolute positioning or CSS ancestry.

## Data and event flow

`DesktopComponent` supplies existing values directly:

- menu key, app title, route categories, language, and clock;
- user identity, task windows, focus id, application metadata, running apps,
  groups, and dock drop-zone state.

The child components emit the original event plus the selected key, category,
window id, app id, or group. `DesktopComponent` immediately delegates those
outputs to the existing `openMb`, `hoverMb`, `shellCat`, `openTray`,
`toggleSession`, `taskClick`, `dockClick`, `dockCtx`, `toggleGroup`, and
`groupCtx` methods. No business logic moves into the children.

Shared `ShellUserIdentity`, `ShellWindowGroup`, and event request types replace
the anonymous shapes currently repeated between state and templates.

## Styling

Move only selectors exclusively owned by the menu bar or taskbar/dock into the
matching component stylesheet. Keep shared selectors such as `.ava`, mode
visibility, light-mode adaptation, reduced-motion rules, window-layer
reservations, and session-menu styling in `desktop.component.scss`.

The extracted styles remain globally scoped through `ViewEncapsulation.None`
because existing selectors depend on the outer `#os` attributes.

## Verification

- Component tests exercise rendered labels and real output events.
- Existing `DesktopComponent` route-shell tests remain the integration
  compatibility gate.
- A focused Angular test run covers all three specs.
- A production Angular build and `git diff --check` complete the tranche.

