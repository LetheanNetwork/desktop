# Desktop Overlay Modularisation Design

## Goal

Reduce the remaining desktop-shell template and stylesheet concentration by
extracting its overlay chrome into focused Angular components without changing
any control, copy, interaction, state transition, positioning rule, route, or
backend behaviour.

## Compatibility contract

The current `DesktopComponent` behaviour remains the product contract:

- the Start/session menu keeps every category, child app, identity, lock,
  switch-user, log-out, and shut-down control;
- context menus keep their headings, separators, nested menus, positioning,
  hover behaviour, and existing action callbacks;
- language, network, battery, and clock tray panels keep their current content
  and launch actions;
- notifications retain their order, four-item limit, timed dismissal, manual
  dismissal, and taskbar-edge positioning;
- the command palette retains its Meta/OS-key toggle, query filtering, keyboard
  navigation, focus restoration, backdrop close, commands, and copy;
- `DesktopComponent` continues to own all state and actions during this
  extraction;
- no session screen, About dialog, window interaction, NgRx, Wails/CoreGO,
  persistence, fixture, or live-data behaviour changes in this tranche.

## Component boundaries

Create five standalone Angular 22 presentation components under
`frontend-ng/src/app/desktop/shell/`:

1. `ShellStartMenu` renders the two-column Start/session menu and its routed
   child submenu.
2. `ShellContextMenu` renders context-menu items and one nested submenu.
3. `ShellTrayPanel` renders the four menu-bar tray variants.
4. `ShellNotificationStack` renders transient notification cards.
5. `ShellCommandPalette` renders the palette, results, keyboard-help footer,
   query input, and the existing empty-query time/throughput/power widgets.

Each component uses required signal inputs, function-based outputs, `OnPush`,
`ViewEncapsulation.None`, and a `display: contents` host. Mutable legacy state
is bound as primitive and array fields rather than as whole objects, ensuring
`OnPush` children are refreshed without changing the coordinator's state
semantics. Children emit typed interaction intents with the original DOM event
when the coordinator needs it. They do not execute window, session, route,
persistence, or backend actions.

`shell.types.ts` becomes the shared contract location for context items,
notifications, commands, panel state, submenu state, session actions, language
rows, and world clocks. This removes anonymous state shapes from
`DesktopComponent` while keeping its data ownership unchanged.

## DOM handles

Positioning and focus restoration remain coordinator responsibilities.
`ShellStartMenu`, `ShellContextMenu`, and `ShellTrayPanel` expose their rendered
panel element through a read-only getter. `ShellCommandPalette` exposes only a
`focusInput()` method. `DesktopComponent` queries the child instances instead
of reaching through their templates.

These narrow handles avoid moving layout policy into presenters while keeping
their internal DOM private.

## Styling

The shared `.menu`, `.mi`, `.mhead`, `.msep`, `.submenu`, `.chev`, `.ava`, mode,
light-mode, reduced-motion, and focus-ring rules remain in
`desktop.component.scss`.

Selectors owned only by one extracted surface move to its component stylesheet:

- Start menu: `.startpanel`, `.sm-*`, `.sp-*`;
- command palette: `.palette*`, `.pl-*`, and `.plw*`;
- tray panel: `.traypanel`, `.tr-*`, `.trh`;
- notification stack: `.notifs`, `.notif`, `.ni`, `.nt`, `.nx`, and
  `notifIn`;
- context menu has no duplicated base styles and therefore needs only its
  component boundary stylesheet.

Global selectors remain valid through `ViewEncapsulation.None`; no wrapper may
alter the current absolute-positioning ancestry.

## Data and event flow

The parent supplies current values directly. Outputs delegate to the existing
methods:

- Start menu → category toggle, `startLaunch`, `onProg`, `startSub`, and
  `sessionAction`;
- context menu → `openCtxSub`, submenu close, and `runCtx`;
- tray panel → `setLang` and `trayOpen`;
- notification stack → `dismiss`;
- command palette → `onPlInput`, `plKey`, `runCmd`, selected-row update, and
  `palClick`.

No output invents a second state store. Existing objects remain mutable in this
tranche because changing the state model would combine refactoring with a
behavioural migration.

## Verification

- Write each presenter test before its component exists and observe the
  missing-component failure.
- Presenter tests render real controls and assert real output values and DOM
  events.
- The existing `DesktopComponent` route-shell spec remains the integration
  compatibility gate.
- Run all five presenter specs and the desktop spec together.
- Run the Angular production build and `git diff --check`.
- Confirm the user-owned `.playwright-mcp/` directory remains untouched.
