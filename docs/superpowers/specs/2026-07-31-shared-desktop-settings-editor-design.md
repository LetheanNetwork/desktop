<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Shared desktop settings editor design

## Outcome

Control's Configuration surface and the Settings application will render the
same typed desktop-control editor. The existing NgRx store and RxJS effects
remain the state owner and asynchronous synchronisation boundary; components
only select state and dispatch typed actions.

## Architecture

- `desktopControlsFeature` remains the single renderer-side source of truth for
  committed controls, the current draft, loading/saving failures, and
  restart-required feedback.
- `DesktopControlsEffects` remains the only Angular layer that calls
  `DesktopControlsBridgeService`. Its RxJS streams serialise `SetMany` writes
  and project committed snapshots into renderer preferences. A future backend
  push listener can dispatch the existing load/success actions without
  changing either application surface.
- A shared standalone `DesktopControlsPanelView` reads NgRx selectors through
  `Store.selectSignal` for Angular rendering and dispatches typed NgRx actions
  for edits, apply, discard, reset, and retry. Signals are view adapters; they
  do not replace the RxJS store.
- Settings keeps its curated appearance controls and may exclude those keys
  from the generic panel. Control presents the complete catalogue through its
  existing Configuration route and layout wrapper.
- Offline mode continues through `DesktopControlsBridgeService`'s isolated
  in-memory snapshot and makes no Wails request.

## Data flow

```text
Go appconfig.Service.Settings / SetMany
                ^
                | DesktopControlsBridgeService
                | RxJS NgRx effects
                v
desktopControlsFeature <---- typed actions ---- UI surfaces
                |
                +---- selectors ----> shared controls panel
```

The complete valid draft is committed once through `SetMany`. Failed writes
leave the committed store snapshot intact, clear the optimistic draft according
to the existing reducer contract, and expose the bounded error. Successful
writes replace the store snapshot and report controls that require restart.

## UX and accessibility

- Preserve Settings' curated appearance, layout, permission, Reset, Discard,
  and Apply controls.
- Preserve Control's Configuration navigation and provide the same explicit
  actions there.
- Preserve Control's configuration precedence, setting keys, environment
  override guidance, and a truthful live/loading/unavailable/demo badge derived
  from the settings transport and store.
- Render toggle controls as labelled button groups and select/number/text
  controls with accessible labels.
- Disable editing and draft actions while saving; expose failures with
  `role="alert"` and restart feedback with `role="status"`.
- Keep native-permission requests available only in Settings and only after an
  explicit user action.

## Retirement scope

After both surfaces use the shared store, remove the Control-only settings
fixture types, demo data, live-snapshot mapping, and inert `commit-settings`
intent. Do not change Control's model, run, power, system, or services UX.

## Verification

- A focused shared-panel test proves typed edits and explicit draft actions
  dispatch through NgRx.
- Settings tests prove curated controls, permissions, and the shared panel
  retain their behaviour.
- Control tests prove its Configuration route renders the NgRx catalogue and
  dispatches one atomic apply action rather than using live-data fixtures.
- Desktop live-data tests prove Control no longer performs a duplicate settings
  read.
- Run focused Angular tests, Prettier, the production Angular build, and
  `git diff --check`.
