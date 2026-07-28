# Window Interaction Service Design

## Goal

Extract the desktop shell's drag, resize, snap, marquee-selection, group-drag,
and window-group transition algorithms from `DesktopComponent` into one tested
Angular service without removing or redesigning any UX feature.

## Compatibility contract

The current desktop behaviour remains the product contract:

- pointer listeners still begin from the existing wallpaper, titlebar, resize
  handle, and selected-window surfaces;
- only the windowed desktop/full-device layouts permit pointer window
  interactions;
- marquee selection keeps its four-pixel movement threshold and ignores
  minimised or grouped windows;
- a multi-selection moves as one and creates a dock group only when released
  over the dock;
- drag movement keeps 60 pixels horizontally and 40 pixels vertically within
  the window layer;
- resize keeps the current 360-by-260-pixel minimum;
- the 22-pixel snap threshold, corner precedence, half/quarter geometry,
  top-edge maximise behaviour, keyboard transition table, and pre-snap restore
  geometry stay unchanged;
- group creation, collapse/restore, split, close-all, focus, z-order, names,
  application lists, and persistence stay unchanged;
- no markup, styling, route, app surface, Wails/CoreGO transport, or native
  window behaviour changes in this tranche.

## Approaches considered

### Stateless interaction engine — selected

An injectable, root-provided `WindowInteractionService` receives immutable
window snapshots, pointer coordinates, rectangles, and group state. It returns
new window/group snapshots plus transient overlay values. The service performs
no DOM queries, event registration, persistence, routing, or NgRx dispatch.

This gives every algorithm a deterministic unit-test seam while preserving
`DesktopComponent` as the browser-event coordinator and
`WindowManagerService`/NgRx as the durable state owner.

### Stateful pointer-controller service

The service could register global pointer listeners and own marquee, snap
preview, selection, and group proxy state. That would make the parent smaller,
but it would also give a root singleton DOM lifetime and a second state owner.
Teardown, multiple desktop hosts, and tests would become harder.

### Add the algorithms to `WindowManagerService`

This would reduce the number of services, but it would mix pointer geometry and
dock grouping policy into the NgRx facade. The window manager would then depend
on DOM coordinate concepts despite also serving non-windowed shell and device
views.

## Service boundary

Create `frontend/src/app/desktop/window-interaction.service.ts`.

The service exposes typed sessions and results for:

- normalised marquee rectangles and intersecting window ids;
- drag offsets, constrained positions, edge/corner snap zones, and snap-preview
  geometry;
- constrained resize dimensions;
- selected-window group movement and dock-hit proxy state;
- half, quarter, and maximise snap rectangles;
- applying, changing, and removing snap state while preserving restore
  geometry;
- keyboard-snap intent (`snap`, `unsnap`, or `minimise`);
- immutable create, toggle, split, and close group transitions.

`Win` gains a typed optional `snapState` field because it is already active
runtime state. Existing persisted-window behaviour is not expanded by this
refactor.

## Coordinator responsibilities

`DesktopComponent` retains only responsibilities that require the rendered
browser or other application services:

1. reject an interaction when the current mode, mouse button, target, or
   session does not permit it;
2. read `DOMRect` values and attach/remove `pointermove` and `pointerup`
   listeners;
3. ask the interaction service for the next immutable state;
4. apply windows and focus through `WindowManagerService`;
5. update the existing `marquee`, `snap`, `proxy`, `selected`, and `groups`
   presentation fields;
6. localise a new group name and call the existing persistence boundary.

Small coordinator methods such as `snapWin`, `unsnap`, and `kbSnap` remain
call sites, but contain no geometry or transition table.

## Data flow

### Marquee

The component captures the pointer origin and host rectangle. On movement it
passes those values and current window rectangles to the service. The service
returns the overlay frame, whether the movement crossed the threshold, and the
selected ids. Pointer release hides the overlay and retains the current
click-without-drag clearing behaviour.

### Drag, resize, and snap

The component creates a typed session at pointer down. Each move passes the
session, current pointer, and latest window snapshot to the service. Drag
returns a moved window and snap preview; resize returns a resized window.
Release applies an optional snap result and persists once.

Keyboard snapping asks the service for an intent. The component delegates the
intent to its existing minimise, unsnap, or snap coordinator.

### Grouping

A group-drag session records selected ids and original positions. Each move
returns an immutable window list and dock proxy. Releasing over the dock calls
the service's create transition.

All group actions consume one `WindowGroupingState` snapshot and return a new
snapshot. The component applies it and persists. A z-index allocator callback
preserves the window manager's existing monotonically increasing z-order.

## Error and edge handling

- Invalid group ids and groups with fewer than two requested members are
  no-ops.
- A missing dock rectangle produces `over: false`.
- Unknown or absent keyboard-snap transitions resolve to `unsnap`, matching
  the current table behaviour.
- Geometry calculations deliberately retain the current dimensions and
  thresholds rather than adding new configuration.
- Service methods do not mutate their input windows, groups, sessions, or
  rectangles.

## Testing

Write `window-interaction.service.spec.ts` before the service exists and observe
the missing-module failure. Use the real injected service and hand-derived
literal geometry; no window-manager or DOM mocks are required.

Tests cover:

- marquee normalisation, threshold, visibility/group filtering, and
  intersection;
- drag constraints, snap-zone precedence, preview coordinates, and clearing
  transient snap state;
- resize growth and minimum dimensions;
- every snap rectangle plus pre-snap geometry across re-snapping and unsnap;
- representative keyboard transition branches for snap, unsnap, and minimise;
- selected-window movement and dock hit testing;
- immutable create, restore/collapse, split, and close group transitions.

The existing `DesktopComponent` and `WindowManagerService` specs remain
integration gates. Final verification runs the focused interaction/desktop
specs, the complete frontend CI suite, the Angular production build, and
`git diff --check`.
