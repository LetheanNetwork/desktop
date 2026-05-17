// Defence-in-depth bound-checking for `_loadFromBackend()` response
// shapes. The backend should already cap response sizes; this layer
// protects the renderer from a malformed/oversized/nested-too-deep
// payload (Mantis #1491 MED, wave-2 finding). A view that does not
// bound-check can blow up Lit's diff loop with a 10K-row array, OOM
// the renderer on a 50MB string field, or render garbage when the
// backend returns the wrong shape.
//
// Usage:
//   import { clampRows, boundedRows, clampString } from "@ui/views/_bounds";
//
//   // Length-cap only (no per-row shape validation):
//   const rows = clampRows(r?.Value?.contacts);
//   if (rows.length > 0) this.contacts = rows as Contact[];
//
//   // Shape-validating: drops rows that fail the predicate, caps the
//   // array, clamps string fields inside the validator.
//   const rows = boundedRows(r?.Value?.contacts, (raw) => {
//     if (!isRecord(raw)) return null;
//     const name = clampString(raw.name);
//     if (!name) return null;
//     return { name, role: clampString(raw.role ?? "") };
//   });

/** Cap on rows per response. Generous default — sales pipelines can
 *  legitimately carry several hundred deals; mail folders a few thousand
 *  threads. Tune per-view via the `maxRows` arg when the legitimate
 *  upper bound is known. */
export const DEFAULT_MAX_ROWS = 2000;

/** Cap on bytes per string field. 64KB swallows reasonable note bodies
 *  and HTML snippets; anything larger is almost certainly the backend
 *  returning a binary blob in a string field by accident. */
export const DEFAULT_MAX_FIELD_BYTES = 64_000;

/** Cap on fields per row. Defends against `Object.keys()` blow-ups on
 *  an attacker-supplied row whose every key has been weaponised. */
export const DEFAULT_MAX_FIELDS_PER_ROW = 50;

/** True for plain objects we can safely index. Filters out arrays,
 *  null, Date, Map, Set, etc — all of which would defeat `obj.foo`
 *  lookups by returning unexpected types. */
export function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object"
    && v !== null
    && !Array.isArray(v)
    && Object.getPrototypeOf(v) === Object.prototype;
}

/** Length-cap an array. Returns `[]` if `rows` isn't an array (defends
 *  against backend returning `null`, an object, or a primitive). Caller
 *  is responsible for any per-row shape narrowing.
 *
 *  Use this when the row shape comes from a typed wails binding the
 *  caller already trusts and only the array-length attack surface
 *  needs defence. */
export function clampRows<T>(rows: unknown, maxRows: number = DEFAULT_MAX_ROWS): T[] {
  if (!Array.isArray(rows)) return [];
  if (rows.length <= maxRows) return rows as T[];
  return (rows as T[]).slice(0, maxRows);
}

/** Length-cap + per-row validate. Validator returns the narrowed row
 *  on success, `null` to drop the row. Dropped rows do NOT count
 *  against `maxRows` (the cap is on input width, not output). */
export function boundedRows<T>(
  rows: unknown,
  validate: (row: unknown) => T | null,
  maxRows: number = DEFAULT_MAX_ROWS,
): T[] {
  if (!Array.isArray(rows)) return [];
  const src = rows.length > maxRows ? rows.slice(0, maxRows) : rows;
  const out: T[] = [];
  for (const row of src) {
    const v = validate(row);
    if (v !== null) out.push(v);
  }
  return out;
}

/** Clamp a string field by UTF-8 byte size. Returns `""` for non-string
 *  inputs (defends against backend returning a number/object where a
 *  string was expected). Truncates at a code-unit boundary that does
 *  not split a surrogate pair so the result is always valid JS string.
 *  Truncation is silent — the renderer gets a shortened value, not an
 *  error. */
export function clampString(v: unknown, maxBytes: number = DEFAULT_MAX_FIELD_BYTES): string {
  if (typeof v !== "string") return "";
  // Fast path — ASCII-only strings fit in `length` bytes.
  if (v.length <= maxBytes) return v;
  // Slow path — re-encode + slice. TextEncoder gives us the actual
  // byte length without instantiating a Buffer.
  const enc = new TextEncoder();
  const bytes = enc.encode(v);
  if (bytes.length <= maxBytes) return v;
  // Slice the byte buffer then decode with the `fatal: false` default
  // so a split multi-byte sequence at the boundary degrades to U+FFFD
  // rather than throwing. Then trim any trailing replacement char so
  // the user never sees the boundary artefact.
  const slice = bytes.slice(0, maxBytes);
  const out = new TextDecoder("utf-8").decode(slice);
  return out.replace(/�+$/, "");
}

/** Cap field-count on a record. Returns a new object with at most
 *  `maxFields` own enumerable keys (insertion-order preserved). Defends
 *  against an attacker-supplied row with 10K junk keys that would
 *  pathologically slow downstream `Object.entries`/`for-in` loops. */
export function clampFields(
  v: unknown,
  maxFields: number = DEFAULT_MAX_FIELDS_PER_ROW,
): Record<string, unknown> {
  if (!isRecord(v)) return {};
  const keys = Object.keys(v);
  if (keys.length <= maxFields) return v;
  const out: Record<string, unknown> = {};
  for (let i = 0; i < maxFields; i++) out[keys[i]] = v[keys[i]];
  return out;
}
