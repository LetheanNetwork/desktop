import { Injectable } from '@angular/core';
import type { Win, WindowSnapState } from './desktop.data';
import type { ShellWindowGroup } from './shell/shell.types';

const MARQUEE_THRESHOLD = 4;
const SNAP_THRESHOLD = 22;
const MINIMUM_VISIBLE_WIDTH = 60;
const MINIMUM_VISIBLE_HEIGHT = 40;
const MINIMUM_WINDOW_WIDTH = 360;
const MINIMUM_WINDOW_HEIGHT = 260;

export interface InteractionPoint {
  readonly x: number;
  readonly y: number;
}

export interface InteractionRect {
  readonly left: number;
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
  readonly width: number;
  readonly height: number;
}

export interface MarqueeSession {
  readonly origin: InteractionPoint;
  readonly hostLeft: number;
  readonly hostTop: number;
}

export interface MarqueeCandidate {
  readonly id: string;
  readonly min: boolean;
  readonly group?: string;
  readonly rect: InteractionRect;
}

export interface MarqueeUpdate {
  readonly marquee: {
    readonly open: true;
    readonly left: number;
    readonly top: number;
    readonly w: number;
    readonly h: number;
  };
  readonly selectedIds: string[];
  readonly moved: boolean;
}

export interface DragSession {
  readonly windowId: string;
  readonly offsetX: number;
  readonly offsetY: number;
  readonly layer: InteractionRect;
  readonly previewOffsetX: number;
  readonly previewOffsetY: number;
}

export interface SnapPreview {
  readonly zone: WindowSnapState | null;
  readonly left: number;
  readonly top: number;
  readonly w: number;
  readonly h: number;
}

export interface DragUpdate {
  readonly window: Win;
  readonly snap: SnapPreview;
}

export interface ResizeSession {
  readonly windowId: string;
  readonly pointerOrigin: InteractionPoint;
  readonly width: number;
  readonly height: number;
}

interface GroupDragOrigin {
  readonly id: string;
  readonly x: number;
  readonly y: number;
}

export interface GroupDragSession {
  readonly ids: string[];
  readonly pointerOrigin: InteractionPoint;
  readonly windows: GroupDragOrigin[];
  readonly hostLeft: number;
  readonly hostTop: number;
}

export interface GroupProxy {
  readonly open: true;
  readonly left: number;
  readonly top: number;
  readonly n: number;
  readonly over: boolean;
}

export interface GroupDragUpdate {
  readonly windows: Win[];
  readonly proxy: GroupProxy;
}

export type KeyboardSnapDirection = 'left' | 'right' | 'up' | 'down';

export type KeyboardSnapIntent =
  | { readonly kind: 'snap'; readonly zone: WindowSnapState }
  | { readonly kind: 'unsnap' }
  | { readonly kind: 'minimise' };

export interface WindowGroupingState {
  readonly windows: readonly Win[];
  readonly groups: readonly ShellWindowGroup[];
  readonly focusId: string | null;
  readonly selectedIds: readonly string[];
}

export interface WindowBounds {
  readonly width: number;
  readonly height: number;
}

export interface SnapRect {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly max: boolean;
}

type KeyboardSnapState = WindowSnapState | 'none';
type KeyboardSnapTarget = WindowSnapState | null | '__min';

const KEYBOARD_SNAP: Record<
  KeyboardSnapDirection,
  Partial<Record<KeyboardSnapState, KeyboardSnapTarget>>
> = {
  left: {
    none: 'left',
    right: null,
    left: 'left',
    tr: 'tl',
    br: 'bl',
    tl: 'tl',
    bl: 'bl',
    max: 'left',
  },
  right: {
    none: 'right',
    left: null,
    right: 'right',
    tl: 'tr',
    bl: 'br',
    tr: 'tr',
    br: 'br',
    max: 'right',
  },
  up: {
    none: 'max',
    left: 'tl',
    right: 'tr',
    bl: 'left',
    br: 'right',
    tl: 'tl',
    tr: 'tr',
    max: 'max',
  },
  down: {
    none: '__min',
    left: null,
    right: null,
    tl: 'left',
    tr: 'right',
    bl: null,
    br: null,
    max: null,
  },
};

@Injectable({ providedIn: 'root' })
export class WindowInteractionService {
  beginMarquee(origin: InteractionPoint, host: InteractionRect): MarqueeSession {
    return {
      origin: { ...origin },
      hostLeft: host.left,
      hostTop: host.top,
    };
  }

  moveMarquee(
    session: MarqueeSession,
    pointer: InteractionPoint,
    candidates: readonly MarqueeCandidate[],
  ): MarqueeUpdate {
    const left = Math.min(session.origin.x, pointer.x);
    const top = Math.min(session.origin.y, pointer.y);
    const width = Math.abs(pointer.x - session.origin.x);
    const height = Math.abs(pointer.y - session.origin.y);
    const selection = {
      left,
      top,
      right: left + width,
      bottom: top + height,
    };

    return {
      marquee: {
        open: true,
        left: left - session.hostLeft,
        top: top - session.hostTop,
        w: width,
        h: height,
      },
      selectedIds: candidates
        .filter(
          (candidate) =>
            !candidate.min && !candidate.group && this.intersects(candidate.rect, selection),
        )
        .map((candidate) => candidate.id),
      moved: width > MARQUEE_THRESHOLD || height > MARQUEE_THRESHOLD,
    };
  }

  beginDrag(
    window: Win,
    pointer: InteractionPoint,
    layer: InteractionRect,
    host: InteractionRect,
  ): DragSession {
    return {
      windowId: window.id,
      offsetX: pointer.x - (layer.left + window.x),
      offsetY: pointer.y - (layer.top + window.y),
      // Not a spread: callers hand this a DOMRect, whose fields live on the
      // prototype as accessors, so {...rect} copies nothing and every later
      // moveDrag reads undefined and returns NaN — the window never moves.
      layer: {
        left: layer.left,
        top: layer.top,
        right: layer.right,
        bottom: layer.bottom,
        width: layer.width,
        height: layer.height,
      },
      previewOffsetX: layer.left - host.left,
      previewOffsetY: layer.top - host.top,
    };
  }

  moveDrag(session: DragSession, pointer: InteractionPoint, window: Win): DragUpdate {
    const x = this.clamp(
      pointer.x - session.layer.left - session.offsetX,
      0,
      session.layer.width - MINIMUM_VISIBLE_WIDTH,
    );
    const y = this.clamp(
      pointer.y - session.layer.top - session.offsetY,
      0,
      session.layer.height - MINIMUM_VISIBLE_HEIGHT,
    );
    const zone = this.snapZone(pointer, session.layer);

    return {
      window: {
        ...window,
        x,
        y,
        snapState: null,
      },
      snap: zone ? this.snapPreview(zone, session) : { zone: null, left: 0, top: 0, w: 0, h: 0 },
    };
  }

  beginResize(window: Win, pointer: InteractionPoint): ResizeSession {
    return {
      windowId: window.id,
      pointerOrigin: { ...pointer },
      width: window.w,
      height: window.h,
    };
  }

  moveResize(session: ResizeSession, pointer: InteractionPoint, window: Win): Win {
    return {
      ...window,
      w: Math.max(MINIMUM_WINDOW_WIDTH, session.width + pointer.x - session.pointerOrigin.x),
      h: Math.max(MINIMUM_WINDOW_HEIGHT, session.height + pointer.y - session.pointerOrigin.y),
    };
  }

  beginGroupDrag(
    ids: readonly string[],
    windows: readonly Win[],
    pointer: InteractionPoint,
    host: InteractionRect,
  ): GroupDragSession {
    return {
      ids: [...ids],
      pointerOrigin: { ...pointer },
      windows: ids.flatMap((id) => {
        const window = windows.find((candidate) => candidate.id === id);
        return window ? [{ id, x: window.x, y: window.y }] : [];
      }),
      hostLeft: host.left,
      hostTop: host.top,
    };
  }

  moveGroupDrag(
    session: GroupDragSession,
    pointer: InteractionPoint,
    windows: readonly Win[],
    dock: InteractionRect | null,
  ): GroupDragUpdate {
    const dx = pointer.x - session.pointerOrigin.x;
    const dy = pointer.y - session.pointerOrigin.y;
    const origins = new Map(session.windows.map((window) => [window.id, window]));
    const over =
      dock !== null &&
      pointer.x >= dock.left &&
      pointer.x <= dock.right &&
      pointer.y >= dock.top &&
      pointer.y <= dock.bottom;

    return {
      windows: windows.map((window) => {
        const origin = origins.get(window.id);
        return origin ? { ...window, x: origin.x + dx, y: origin.y + dy } : window;
      }),
      proxy: {
        open: true,
        left: pointer.x - session.hostLeft + 14,
        top: pointer.y - session.hostTop + 14,
        n: session.ids.length,
        over,
      },
    };
  }

  snapRect(zone: WindowSnapState, bounds: WindowBounds): SnapRect {
    const halfWidth = bounds.width / 2;
    const halfHeight = bounds.height / 2;

    switch (zone) {
      case 'top':
      case 'max':
        return { x: 0, y: 0, w: bounds.width, h: bounds.height, max: true };
      case 'left':
        return { x: 0, y: 0, w: halfWidth, h: bounds.height, max: false };
      case 'right':
        return { x: halfWidth, y: 0, w: halfWidth, h: bounds.height, max: false };
      case 'tl':
        return { x: 0, y: 0, w: halfWidth, h: halfHeight, max: false };
      case 'tr':
        return { x: halfWidth, y: 0, w: halfWidth, h: halfHeight, max: false };
      case 'bl':
        return { x: 0, y: halfHeight, w: halfWidth, h: halfHeight, max: false };
      case 'br':
        return {
          x: halfWidth,
          y: halfHeight,
          w: halfWidth,
          h: halfHeight,
          max: false,
        };
    }
  }

  snapWindow(window: Win, zone: WindowSnapState, bounds: WindowBounds): Win {
    const previous = window.snapState
      ? window.prev
      : { x: window.x, y: window.y, w: window.w, h: window.h };

    return {
      ...window,
      ...this.snapRect(zone, bounds),
      prev: previous,
      snapState: zone,
    };
  }

  unsnapWindow(window: Win): Win {
    return {
      ...window,
      ...(window.prev ?? {}),
      max: false,
      snapState: null,
    };
  }

  keyboardSnap(window: Win, direction: KeyboardSnapDirection): KeyboardSnapIntent {
    const state: KeyboardSnapState = window.snapState ?? 'none';
    const target = KEYBOARD_SNAP[direction][state];

    if (target === '__min') return { kind: 'minimise' };
    if (target === null || target === undefined) return { kind: 'unsnap' };
    return { kind: 'snap', zone: target };
  }

  createGroup(
    state: WindowGroupingState,
    ids: readonly string[],
    name: string,
    id = 'g' + Date.now(),
  ): WindowGroupingState | null {
    if (ids.length < 2) return null;

    const apps = ids.flatMap((windowId) => {
      const window = state.windows.find((candidate) => candidate.id === windowId);
      return window ? [window.app] : [];
    });

    return {
      windows: state.windows.map((window) =>
        ids.includes(window.id) ? { ...window, min: true, group: id } : window,
      ),
      groups: [
        ...state.groups,
        {
          id,
          name,
          ids: [...ids],
          apps,
          open: false,
        },
      ],
      selectedIds: [],
      focusId: state.focusId && ids.includes(state.focusId) ? null : state.focusId,
    };
  }

  toggleGroup(
    state: WindowGroupingState,
    id: string,
    nextZ: () => number,
  ): WindowGroupingState | null {
    const group = state.groups.find((candidate) => candidate.id === id);
    if (!group) return null;

    const open = !group.open;
    return {
      windows: state.windows.map((window) =>
        group.ids.includes(window.id)
          ? {
              ...window,
              min: !open,
              z: open ? nextZ() : window.z,
            }
          : window,
      ),
      groups: state.groups.map((candidate) =>
        candidate.id === id ? { ...candidate, open } : candidate,
      ),
      selectedIds: [...state.selectedIds],
      focusId: open
        ? (group.ids[group.ids.length - 1] ?? null)
        : state.focusId && group.ids.includes(state.focusId)
          ? null
          : state.focusId,
    };
  }

  splitGroup(
    state: WindowGroupingState,
    id: string,
    nextZ: () => number,
  ): WindowGroupingState | null {
    const group = state.groups.find((candidate) => candidate.id === id);
    if (!group) return null;

    return {
      windows: state.windows.map((window) =>
        group.ids.includes(window.id)
          ? {
              ...window,
              group: undefined,
              z: group.open ? nextZ() : window.z,
            }
          : window,
      ),
      groups: state.groups.filter((candidate) => candidate.id !== id),
      selectedIds: [...state.selectedIds],
      focusId: state.focusId,
    };
  }

  closeGroup(state: WindowGroupingState, id: string): WindowGroupingState | null {
    const group = state.groups.find((candidate) => candidate.id === id);
    if (!group) return null;

    return {
      windows: state.windows.filter((window) => !group.ids.includes(window.id)),
      groups: state.groups.filter((candidate) => candidate.id !== id),
      selectedIds: [...state.selectedIds],
      focusId: state.focusId && group.ids.includes(state.focusId) ? null : state.focusId,
    };
  }

  private intersects(
    left: InteractionRect,
    right: Pick<InteractionRect, 'left' | 'top' | 'right' | 'bottom'>,
  ): boolean {
    return (
      left.left < right.right &&
      left.right > right.left &&
      left.top < right.bottom &&
      left.bottom > right.top
    );
  }

  private snapZone(pointer: InteractionPoint, layer: InteractionRect): WindowSnapState | null {
    const x = pointer.x - layer.left;
    const y = pointer.y - layer.top;
    const nearLeft = x < SNAP_THRESHOLD;
    const nearRight = layer.width - x < SNAP_THRESHOLD;
    const nearTop = y < SNAP_THRESHOLD;
    const nearBottom = layer.height - y < SNAP_THRESHOLD;

    if (nearTop && nearLeft) return 'tl';
    if (nearTop && nearRight) return 'tr';
    if (nearBottom && nearLeft) return 'bl';
    if (nearBottom && nearRight) return 'br';
    if (nearTop) return 'top';
    if (nearLeft) return 'left';
    if (nearRight) return 'right';
    return null;
  }

  private snapPreview(zone: WindowSnapState, session: DragSession): SnapPreview {
    const rect = this.snapRect(zone, session.layer);
    return {
      zone,
      left: session.previewOffsetX + rect.x,
      top: session.previewOffsetY + rect.y,
      w: rect.w,
      h: rect.h,
    };
  }

  private clamp(value: number, minimum: number, maximum: number): number {
    return Math.max(minimum, Math.min(value, maximum));
  }
}
