# Typed Desktop Fixtures Design

## Goal

Move the desktop application catalogue, developer-panel payloads, world-clock
rows, and package-status rows out of `DesktopComponent` and the mixed
`desktop.data.ts` module into focused, typed data files without changing any
rendered content or behaviour.

## Compatibility contract

This is an ownership and typing refactor:

- every application id, title, icon, size, hint, route, category, ordering
  entry, and child navigation row remains unchanged;
- every developer-panel kind, metric, column, row, tree node, log line, card,
  kanban column, terminal line, localisation marker, and route key remains
  unchanged;
- world-clock cities, labels, IANA time zones, order, and formatting remain
  unchanged;
- package names, localised states, status-dot variants, and order remain
  unchanged;
- `DesktopComponent` retains clock formatting, live timer updates, widget
  rendering, route lookup, and app/window coordination;
- `WindowManagerService`, NgRx, Wails/CoreGO transport, routes, templates,
  styles, native windows, and persistence do not change;
- the existing `desktop.data.ts` exports remain available as a compatibility
  facade while production consumers can import from the focused source files.

## Approaches considered

### Three focused typed modules — selected

Create:

1. `desktop-catalogue.data.ts` for application definitions, order, categories,
   and app-internal navigation rails;
2. `dev-panel.data.ts` for developer-panel fixture types, route keys, payloads,
   and lookup;
3. `desktop-shell-fixtures.data.ts` for localised world-clock and package rows.

`desktop.data.ts` keeps core window/data-token types, telemetry, language, and
filesystem fixtures, and re-exports the catalogue and shell rows for existing
imports. This gives each large dataset one discoverable owner without forcing
an unrelated repository-wide import migration.

### One large `desktop-fixtures.data.ts`

A single extraction would make `DesktopComponent` smaller, but would preserve
the current mixture of route metadata, developer payloads, and widget rows.
Future changes would still require opening one broad fixture file.

### Dependency-injected fixture service

An injection token or service could provide all fixture data. That would be
appropriate when CoreGO supplies these payloads live, but it adds runtime
state and provider configuration to a static-data refactor. The existing
`DESKTOP_DATA` token remains the live-payload seam.

## Application catalogue

`desktop-catalogue.data.ts` owns:

- `AppDef` and `Category`;
- `AppNavItem`;
- `APPS`, `ORDER`, and `CATEGORIES`;
- `CTRL_NAV`, `GAMES_NAV`, and `SETTINGS_NAV`.

The existing surface registry remains composable into the catalogue. Its
imports of `AppDef` and `Category` become type-only imports from the catalogue,
so no runtime cycle is introduced.

`desktop-route-tree.ts`, `DesktopComponent`, and direct catalogue consumers
import from this file. `desktop.data.ts` re-exports the same bindings for
compatibility and uses them to construct `DEFAULT_DESKTOP_DATA`.

## Developer-panel fixtures

`dev-panel.data.ts` defines a strict source union:

- table panels with optional metric tiles plus serialised typed columns and
  rows;
- tree panels with typed depth/name/folder tuples;
- console panels with timestamp/source/message tuples;
- terminal panels with string lines;
- card panels with title/subtitle/icon records;
- kanban panels with named string-card columns.

The literal catalogue uses `satisfies Record<DevPanelRoute,
DevPanelFixture>`. This makes missing route fixtures, invalid kinds, malformed
tuples, and invalid payload fields compile-time failures.

Table column and row literals are typed before being serialised to the exact
JSON-string interface required by `<lthn-datatable>`. `devPanelFor(route)`
returns a typed empty view for an unknown route. `DesktopComponent.panelFor`
becomes a small route-to-fixture delegation.

`DevPanelApp` receives a `DevPanelView` rather than `any`. The view type exposes
the optional fields required by Angular's template checker while the stricter
fixture union validates catalogue declarations.

## Shell fixtures

`desktop-shell-fixtures.data.ts` defines:

- `WorldClockFixture` and the localised `CLOCKS` rows;
- `PackageFixture`, `PackageStatusVariant`, and the localised `PKGS` rows.

`DesktopComponent` exposes these readonly constants to its existing template.
It continues to calculate current times with `fmtTz`; the data module does not
own timers, locale formatting, or mutable state.

The duplicate unlocalised clock and package declarations are removed from
`desktop.data.ts`.

## Data flow

```text
desktop-catalogue.data.ts ──┬──> routes / DesktopComponent
                            └──> desktop.data.ts compatibility facade

dev-panel.data.ts ─────────────> DesktopComponent.panelFor ──> DevPanelApp

desktop-shell-fixtures.data.ts ─> DesktopComponent ──> existing widgets/tray
```

No data file queries the DOM, injects Angular services, dispatches NgRx
actions, or mutates its exported rows.

## Testing

Write the focused data specs before the new modules exist and observe the
missing-module failure.

- The catalogue spec verifies that ordered ids and category references resolve
  and that each developer application has a developer-panel fixture.
- The developer-panel spec verifies known/unknown lookup, parses each table's
  column/row JSON, rejects duplicate column keys, and verifies every row key is
  represented by a column.
- The shell-fixture spec verifies unique clock/package names, valid IANA time
  zones, non-empty states, and supported status variants.
- The existing desktop route-shell spec remains the rendered compatibility
  gate.

Final verification runs the focused data/desktop tests, the complete frontend
CI suite, the Angular production build, and `git diff --check`. The
user-owned `.playwright-mcp/` directory remains untouched.
