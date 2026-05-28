// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: three near-identical Set<string> ↔ JSON
// localStorage round-trips lived inline in chat-window + model-browser
// (pinned conversations, pinned local models, pinned HF repos). Same
// shape, same failure modes, same load/save/toggle semantics. Three
// copies hit Snider's "three is the threshold" rule — extract.
//
// Usage:
//
//   const pinnedConvos = createPinnedSet("lthn.chat.pinned");
//   const initial = pinnedConvos.load();           // Set<string>
//   pinnedConvos.save(initial);                    // persist
//   const next = pinnedConvos.toggle(initial, id); // returns new Set
//
// Each call returns a NEW Set instance (Lit's `===` change detection
// only re-renders when reference identity changes — same pattern the
// inline helpers used). Save serialises to a sorted JSON array so
// the stored representation is stable across runs.

export interface PinnedSetStore {
  /** Read the stored set from localStorage. Returns an empty Set
   *  when the key is missing OR the value is malformed (defensive —
   *  preferable to throwing on every page load). */
  load(): Set<string>;
  /** Persist the set as a JSON array (sorted for stable storage).
   *  Silent on quota / disabled-storage errors — the in-memory state
   *  still works for the rest of the session. */
  save(pinned: Set<string>): void;
  /** Return a new Set with `entry` toggled (added if absent, removed
   *  if present). Pure — does not write to storage. Caller decides
   *  whether to call save(). */
  toggle(pinned: Set<string>, entry: string): Set<string>;
}

export function createPinnedSet(key: string): PinnedSetStore {
  return {
    load(): Set<string> {
      try {
        const raw = localStorage.getItem(key);
        if (!raw) return new Set();
        const arr = JSON.parse(raw);
        if (!Array.isArray(arr)) return new Set();
        return new Set(arr.filter((x): x is string => typeof x === "string"));
      } catch {
        return new Set();
      }
    },
    save(pinned: Set<string>): void {
      try {
        localStorage.setItem(key, JSON.stringify(Array.from(pinned).sort()));
      } catch {
        /* in-memory state still works */
      }
    },
    toggle(pinned: Set<string>, entry: string): Set<string> {
      const next = new Set(pinned);
      if (next.has(entry)) next.delete(entry);
      else next.add(entry);
      return next;
    },
  };
}
