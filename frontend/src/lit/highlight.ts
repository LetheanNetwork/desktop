// SPDX-Licence-Identifier: EUPL-1.2

/** Shared search-substring split helper. Used by:
 *  - chat-window's rail row title + snippet rendering
 *  - model-browser's HF result name rendering
 *
 *  Returns one Segment per slice, alternating match/non-match.
 *  Empty query OR empty text → single non-match segment (the whole
 *  string). Query is matched literally — no regex semantics. */

export interface HighlightSegment {
  text: string;
  match: boolean;
}

export function highlightMatch(text: string, query: string): HighlightSegment[] {
  if (!text) return [];
  const q = (query || "").trim();
  if (!q) return [{ text, match: false }];
  const tLow = text.toLowerCase();
  const qLow = q.toLowerCase();
  const out: HighlightSegment[] = [];
  let i = 0;
  while (i < text.length) {
    const at = tLow.indexOf(qLow, i);
    if (at < 0) {
      out.push({ text: text.slice(i), match: false });
      break;
    }
    if (at > i) {
      out.push({ text: text.slice(i, at), match: false });
    }
    out.push({ text: text.slice(at, at + q.length), match: true });
    i = at + q.length;
  }
  return out;
}
