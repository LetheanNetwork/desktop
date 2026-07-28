<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Live desktop bridges implementation plan

> Execute locally on `main` without sub-agents. Keep the existing UI and use
> red-green Angular tests for every behaviour change.

## 1. Establish the typed runtime boundary

**Files**

- Add `frontend/src/app/desktop/desktop-live-data.service.spec.ts`
- Add `frontend/src/app/desktop/desktop-live-data.service.ts`

Write failing tests for explicit offline mode, telemetry, model, benchmark,
process, settings, and Files response parsing. Implement a root service that:

- derives demo mode from `ConnectionManagerService.offline`;
- rejects live calls in demo mode before touching `SurfaceBridgeService`;
- normalises each Wails response into readonly frontend types;
- aggregates Control reads with `Promise.allSettled`.

Run:

```bash
cd frontend
npx ng test --watch=false --include=src/app/desktop/desktop-live-data.service.spec.ts
```

## 2. Make Telemetry live without losing its demo composition

**Files**

- Add `frontend/src/app/desktop/apps/telemetry.app.spec.ts`
- Modify `frontend/src/app/desktop/apps/telemetry.app.ts`

Test that offline mode performs no call, connected mode renders real process
metrics, and failure retains labelled demo data. Keep a bounded sample history
for the sparklines and poll only while the component exists.

Run the focused spec.

## 3. Back Files with the Office Files service

**Files**

- Add `frontend/src/app/desktop/apps/files.app.spec.ts`
- Modify `frontend/src/app/desktop/apps/files.app.ts`

Test demo navigation without bridge calls, live locations/recent rows/disk
usage, location refresh, and a failed live read retaining demo data. Preserve
grid/list switching and the existing file-browser layout.

Run the focused spec.

## 4. Populate Control progressively

**Files**

- Add `frontend/src/app/desktop/apps/control.app.spec.ts`
- Modify `frontend/src/app/desktop/apps/control.app.ts`

Test offline demo mode, partial live aggregation, model and benchmark table
mapping, process telemetry, settings grouping, and honest mixed-data
labelling. Unsupported cards remain visibly demo-backed.

Run the focused spec.

## 5. Reuse the live PTY for the base Terminal route

**Files**

- Add `frontend/src/app/desktop/apps/terminal.app.spec.ts`
- Add `frontend/src/app/desktop/apps/terminal.app.ts`
- Modify `frontend/src/app/desktop/apps/app-view.ts`
- Modify the applicable route/registry spec

Test that offline mode selects the existing typed developer fixture and
connected mode selects the existing `AgentsTerminalSurface`. Do not duplicate
xterm or PTY session logic.

Run the Terminal and registry specs.

## 6. Record missing backend sources

**Files**

- Add `TODO.md`
- Update browser-preview documentation only where the existing offline URL
  needs a demo-mode explanation

List the missing data contracts per application, including model runtime
metrics, power, CPU history, daemon health, general filesystem browsing, file
operations, and watch events.

## 7. Verify the tranche

Run:

```bash
cd frontend
npx ng test --watch=false \
  --include=src/app/desktop/desktop-live-data.service.spec.ts \
  --include=src/app/desktop/apps/telemetry.app.spec.ts \
  --include=src/app/desktop/apps/files.app.spec.ts \
  --include=src/app/desktop/apps/control.app.spec.ts \
  --include=src/app/desktop/apps/terminal.app.spec.ts
npm run build
cd ..
git diff --check
```

Then run `task verify:frontend` when the focused gate is green. Review the
final diff for accidental generated bindings, fixture deletion, or unrelated
changes before committing.
