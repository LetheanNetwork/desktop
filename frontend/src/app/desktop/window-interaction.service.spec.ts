import { TestBed } from '@angular/core/testing';
import type { Win } from './desktop.data';
import type { ShellWindowGroup } from './shell/shell.types';
import {
  InteractionRect,
  WindowGroupingState,
  WindowInteractionService,
} from './window-interaction.service';

const windowFixture = (overrides: Partial<Win> = {}): Win => ({
  id: 'control-window',
  app: 'control',
  sub: 'models',
  x: 200,
  y: 100,
  w: 500,
  h: 400,
  z: 10,
  min: false,
  max: false,
  ...overrides,
});

const rect = (left: number, top: number, width: number, height: number): InteractionRect => ({
  left,
  top,
  right: left + width,
  bottom: top + height,
  width,
  height,
});

const groupingState = (overrides: Partial<WindowGroupingState> = {}): WindowGroupingState => ({
  windows: [
    windowFixture({ id: 'control-window', app: 'control', x: 10, y: 20, z: 10 }),
    windowFixture({
      id: 'telemetry-window',
      app: 'telemetry',
      x: 100,
      y: 120,
      z: 11,
    }),
    windowFixture({ id: 'chat-window', app: 'chat', x: 300, y: 80, z: 12 }),
  ],
  groups: [],
  focusId: 'control-window',
  selectedIds: ['control-window', 'telemetry-window'],
  ...overrides,
});

describe('WindowInteractionService', () => {
  let service: WindowInteractionService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(WindowInteractionService);
  });

  it('normalises a reverse marquee and selects only intersecting visible ungrouped windows', () => {
    const session = service.beginMarquee({ x: 300, y: 300 }, rect(100, 50, 800, 600));

    const update = service.moveMarquee(session, { x: 150, y: 120 }, [
      {
        id: 'control-window',
        min: false,
        rect: rect(160, 130, 90, 70),
      },
      {
        id: 'minimised-window',
        min: true,
        rect: rect(170, 140, 60, 50),
      },
      {
        id: 'grouped-window',
        min: false,
        group: 'g1',
        rect: rect(180, 150, 60, 50),
      },
      {
        id: 'outside-window',
        min: false,
        rect: rect(500, 400, 80, 60),
      },
    ]);

    expect(update).toEqual({
      marquee: { open: true, left: 50, top: 70, w: 150, h: 180 },
      selectedIds: ['control-window'],
      moved: true,
    });
  });

  it('keeps the marquee below the four-pixel movement threshold as a click', () => {
    const session = service.beginMarquee({ x: 200, y: 100 }, rect(100, 50, 800, 600));

    expect(service.moveMarquee(session, { x: 204, y: 104 }, [])).toEqual({
      marquee: { open: true, left: 100, top: 50, w: 4, h: 4 },
      selectedIds: [],
      moved: false,
    });
  });

  it('constrains drag geometry, clears snap state, and returns a corner preview', () => {
    const window = windowFixture({
      snapState: 'left',
      prev: { x: 70, y: 40, w: 480, h: 360 },
    });
    const layer = rect(100, 50, 800, 600);
    const session = service.beginDrag(window, { x: 330, y: 180 }, layer, rect(80, 20, 840, 660));

    const corner = service.moveDrag(session, { x: 110, y: 60 }, window);
    const constrained = service.moveDrag(session, { x: 870, y: 640 }, corner.window);

    expect(corner.window).toEqual({
      ...window,
      x: 0,
      y: 0,
      snapState: null,
    });
    expect(corner.snap).toEqual({
      zone: 'tl',
      left: 20,
      top: 30,
      w: 400,
      h: 300,
    });
    expect(constrained.window.x).toBe(740);
    expect(constrained.window.y).toBe(560);
    expect(constrained.snap).toEqual({
      zone: null,
      left: 0,
      top: 0,
      w: 0,
      h: 0,
    });
  });

  it('drags when the layer is a real DOMRect, whose fields are prototype accessors', () => {
    // The literals above spread cleanly, which is exactly how this bug hid: a
    // browser DOMRect keeps left/top/width/height on the prototype, so
    // spreading one into the session copied nothing and every moveDrag
    // returned NaN. This rect reproduces that shape.
    const plain = rect(100, 50, 800, 600);
    const accessorRect = Object.create(
      {},
      Object.fromEntries(
        (['left', 'top', 'right', 'bottom', 'width', 'height'] as const).map((key) => [
          key,
          { get: () => plain[key] },
        ]),
      ),
    ) as InteractionRect;
    const window = windowFixture();

    const session = service.beginDrag(window, { x: 330, y: 180 }, accessorRect, accessorRect);
    const update = service.moveDrag(session, { x: 430, y: 260 }, window);

    expect(update.window.x).toBe(300);
    expect(update.window.y).toBe(180);
  });

  it('arms the tear-off zone once the pointer leaves the shell', () => {
    const window = windowFixture({ w: 400, h: 300 });
    const layer = rect(100, 50, 800, 600);
    const session = service.beginDrag(window, { x: 330, y: 180 }, layer, rect(80, 20, 840, 660));

    const outside = service.moveDrag(session, { x: 960, y: 400 }, window);

    expect(outside.tear).toEqual({
      active: true,
      left: 440,
      top: 350,
      w: 400,
      h: 300,
    });
    expect(outside.snap.zone).toBeNull();
  });

  it('leaves the snap margins to snapping and never tears off inside them', () => {
    const window = windowFixture();
    const layer = rect(100, 50, 800, 600);
    const session = service.beginDrag(window, { x: 330, y: 180 }, layer, rect(100, 50, 800, 600));

    // Every margin the snap zones own, walked from the inside.
    const margins: [name: string, x: number, y: number][] = [
      ['top-left', 105, 55],
      ['top-right', 895, 55],
      ['bottom-left', 105, 645],
      ['bottom-right', 895, 645],
      ['top', 500, 55],
      ['left', 105, 300],
      ['right', 895, 300],
    ];

    for (const [, x, y] of margins) {
      const update = service.moveDrag(session, { x, y }, window);
      expect(update.snap.zone).not.toBeNull();
      expect(update.tear.active).toBe(false);
    }
  });

  it('tears off past the edge the snap margin would otherwise have claimed', () => {
    const window = windowFixture();
    const layer = rect(100, 50, 800, 600);
    const session = service.beginDrag(window, { x: 330, y: 180 }, layer, rect(100, 50, 800, 600));

    const justInside = service.moveDrag(session, { x: 100, y: 300 }, window);
    const justOutside = service.moveDrag(session, { x: 99, y: 300 }, window);

    expect(justInside.snap.zone).toBe('left');
    expect(justInside.tear.active).toBe(false);
    expect(justOutside.snap.zone).toBeNull();
    expect(justOutside.tear.active).toBe(true);
  });

  it('holds the tear-off preview inside the shell so its label stays readable', () => {
    const window = windowFixture({ w: 1200, h: 900 });
    const layer = rect(100, 50, 800, 600);
    const session = service.beginDrag(window, { x: 330, y: 180 }, layer, rect(100, 50, 800, 600));

    const update = service.moveDrag(session, { x: -400, y: -400 }, window);

    expect(update.tear).toEqual({ active: true, left: 0, top: 0, w: 800, h: 600 });
  });

  it('grows a resize from its pointer origin and enforces the existing minimum', () => {
    const window = windowFixture();
    const session = service.beginResize(window, { x: 500, y: 400 });

    const minimum = service.moveResize(session, { x: 300, y: 200 }, window);
    const grown = service.moveResize(session, { x: 650, y: 525 }, window);

    expect(minimum.w).toBe(360);
    expect(minimum.h).toBe(260);
    expect(grown.w).toBe(650);
    expect(grown.h).toBe(525);
  });

  it('returns the exact maximise, half, and quarter snap rectangles', () => {
    const bounds = { width: 1000, height: 700 };

    expect(service.snapRect('top', bounds)).toEqual({
      x: 0,
      y: 0,
      w: 1000,
      h: 700,
      max: true,
    });
    expect(service.snapRect('max', bounds)).toEqual({
      x: 0,
      y: 0,
      w: 1000,
      h: 700,
      max: true,
    });
    expect(service.snapRect('left', bounds)).toEqual({
      x: 0,
      y: 0,
      w: 500,
      h: 700,
      max: false,
    });
    expect(service.snapRect('right', bounds)).toEqual({
      x: 500,
      y: 0,
      w: 500,
      h: 700,
      max: false,
    });
    expect(service.snapRect('tl', bounds)).toEqual({
      x: 0,
      y: 0,
      w: 500,
      h: 350,
      max: false,
    });
    expect(service.snapRect('tr', bounds)).toEqual({
      x: 500,
      y: 0,
      w: 500,
      h: 350,
      max: false,
    });
    expect(service.snapRect('bl', bounds)).toEqual({
      x: 0,
      y: 350,
      w: 500,
      h: 350,
      max: false,
    });
    expect(service.snapRect('br', bounds)).toEqual({
      x: 500,
      y: 350,
      w: 500,
      h: 350,
      max: false,
    });
  });

  it('preserves the original restore geometry while changing snap zones', () => {
    const window = windowFixture({ x: 75, y: 45, w: 520, h: 410 });

    const left = service.snapWindow(window, 'left', { width: 1000, height: 700 });
    const right = service.snapWindow(left, 'right', { width: 1000, height: 700 });
    const restored = service.unsnapWindow(right);

    expect(left.prev).toEqual({ x: 75, y: 45, w: 520, h: 410 });
    expect(right.prev).toEqual({ x: 75, y: 45, w: 520, h: 410 });
    expect(restored).toEqual({
      ...right,
      x: 75,
      y: 45,
      w: 520,
      h: 410,
      max: false,
      snapState: null,
    });
  });

  it('maps keyboard snapping to snap, unsnap, and minimise intents', () => {
    expect(service.keyboardSnap(windowFixture({ snapState: null }), 'left')).toEqual({
      kind: 'snap',
      zone: 'left',
    });
    expect(service.keyboardSnap(windowFixture({ snapState: 'tl' }), 'right')).toEqual({
      kind: 'snap',
      zone: 'tr',
    });
    expect(service.keyboardSnap(windowFixture({ snapState: 'max' }), 'up')).toEqual({
      kind: 'snap',
      zone: 'max',
    });
    expect(service.keyboardSnap(windowFixture({ snapState: 'right' }), 'left')).toEqual({
      kind: 'unsnap',
    });
    expect(service.keyboardSnap(windowFixture({ snapState: 'top' }), 'left')).toEqual({
      kind: 'unsnap',
    });
    expect(service.keyboardSnap(windowFixture({ snapState: null }), 'down')).toEqual({
      kind: 'minimise',
    });
  });

  it('moves selected windows together and reports whether the pointer is over the dock', () => {
    const state = groupingState();
    const session = service.beginGroupDrag(
      ['control-window', 'telemetry-window'],
      state.windows,
      { x: 200, y: 200 },
      rect(50, 20, 1200, 800),
    );

    const update = service.moveGroupDrag(
      session,
      { x: 450, y: 720 },
      state.windows,
      rect(400, 700, 300, 80),
    );

    expect(update.windows.map(({ id, x, y }) => ({ id, x, y }))).toEqual([
      { id: 'control-window', x: 260, y: 540 },
      { id: 'telemetry-window', x: 350, y: 640 },
      { id: 'chat-window', x: 300, y: 80 },
    ]);
    expect(update.proxy).toEqual({
      open: true,
      left: 414,
      top: 714,
      n: 2,
      over: true,
    });
  });

  it('creates an immutable group and clears grouped selection and focus', () => {
    const state = groupingState();

    const grouped = service.createGroup(
      state,
      ['control-window', 'telemetry-window'],
      'Group 1',
      'g100',
    );

    expect(grouped).not.toBeNull();
    expect(grouped?.windows.map(({ id, min, group }) => ({ id, min, group }))).toEqual([
      { id: 'control-window', min: true, group: 'g100' },
      { id: 'telemetry-window', min: true, group: 'g100' },
      { id: 'chat-window', min: false, group: undefined },
    ]);
    expect(grouped?.groups).toEqual([
      {
        id: 'g100',
        name: 'Group 1',
        ids: ['control-window', 'telemetry-window'],
        apps: ['control', 'telemetry'],
        open: false,
      },
    ]);
    expect(grouped?.selectedIds).toEqual([]);
    expect(grouped?.focusId).toBeNull();
    expect(state.windows[0]).toEqual(
      windowFixture({ id: 'control-window', app: 'control', x: 10, y: 20, z: 10 }),
    );
    expect(state.groups).toEqual([]);
  });

  it('restores and collapses a group with the existing focus and z-order rules', () => {
    const group: ShellWindowGroup = {
      id: 'g1',
      name: 'Group 1',
      ids: ['control-window', 'telemetry-window'],
      apps: ['control', 'telemetry'],
      open: false,
    };
    const state = groupingState({
      windows: groupingState().windows.map((window) =>
        group.ids.includes(window.id) ? { ...window, min: true, group: group.id } : window,
      ),
      groups: [group],
      focusId: null,
    });
    let z = 20;

    const restored = service.toggleGroup(state, 'g1', () => ++z);
    const collapsed = restored && service.toggleGroup(restored, 'g1', () => ++z);

    expect(restored?.groups[0].open).toBe(true);
    expect(restored?.windows.slice(0, 2).map(({ min, z }) => ({ min, z }))).toEqual([
      { min: false, z: 21 },
      { min: false, z: 22 },
    ]);
    expect(restored?.focusId).toBe('telemetry-window');
    expect(collapsed?.groups[0].open).toBe(false);
    expect(collapsed?.windows.slice(0, 2).map(({ min, z }) => ({ min, z }))).toEqual([
      { min: true, z: 21 },
      { min: true, z: 22 },
    ]);
    expect(collapsed?.focusId).toBeNull();
  });

  it('splits an open group and closes all windows in a group', () => {
    const group: ShellWindowGroup = {
      id: 'g1',
      name: 'Group 1',
      ids: ['control-window', 'telemetry-window'],
      apps: ['control', 'telemetry'],
      open: true,
    };
    const state = groupingState({
      windows: groupingState().windows.map((window) =>
        group.ids.includes(window.id) ? { ...window, group: group.id } : window,
      ),
      groups: [group],
      focusId: 'telemetry-window',
    });
    let z = 30;

    const split = service.splitGroup(state, 'g1', () => ++z);
    const closed = service.closeGroup(state, 'g1');

    expect(split?.groups).toEqual([]);
    expect(split?.windows.slice(0, 2).map(({ group, z }) => ({ group, z }))).toEqual([
      { group: undefined, z: 31 },
      { group: undefined, z: 32 },
    ]);
    expect(split?.focusId).toBe('telemetry-window');
    expect(closed?.windows.map((window) => window.id)).toEqual(['chat-window']);
    expect(closed?.groups).toEqual([]);
    expect(closed?.focusId).toBeNull();
  });

  it('treats undersized and unknown group requests as no-ops', () => {
    const state = groupingState();

    expect(service.createGroup(state, ['control-window'], 'Group 1', 'g1')).toBeNull();
    expect(service.toggleGroup(state, 'missing', () => 20)).toBeNull();
    expect(service.splitGroup(state, 'missing', () => 20)).toBeNull();
    expect(service.closeGroup(state, 'missing')).toBeNull();
  });
});
