// SPDX-Licence-Identifier: EUPL-1.2
//
// bindings-invariants.test.ts — regression pin for the wails3 binding-
// generator's nested-struct marshalling shape per
// plans/code/lthn/desktop/auth-gate/RFC.stage-f.md (Cerberus #55 ADD-1
// / Mantis #1664 Phase A).
//
// Invariant under test:
//
//   The generated `export class Core` in frontend/bindings/dappco.re/go/
//   models.ts MUST contain NO callable instance methods. The wails3
//   generator marshals nested struct fields (e.g. core.Core.Service())
//   as method-less `Partial<>` shapes — when this assumption breaks
//   silently (a future wails3 release flips to method-emit, or a
//   Service-shaped Go symbol leaks across the bindings boundary), the
//   frontend's substrate-fixture assumption fragments without a
//   compile-time signal.
//
// Why this matters: the Phase B sandboxsubstrate refactor relies on
// Core staying a method-less DTO surface. If `.Service()` ever appears
// as a method declaration in the class body, the bindings have started
// exporting RPC handles and the substrate-shim's TierGoOnly gate is
// bypass-able from the frontend without crossing the broker. The pin
// catches the regression at test time rather than at runtime.
//
// Shape of the test:
//
//   1. Read frontend/bindings/dappco.re/go/models.ts
//   2. Locate `export class Core { … }` block
//   3. Assert the class body contains ONLY constructor + static
//      createFrom — no callable instance methods (no `Service()` /
//      `Foo()` / generic-name `\w+\(` patterns at method-position).
//
// SECURITY-NOTE escape valve from the dispatch brief: if the binding
// shape changes in a way that makes the string-match brittle (e.g.
// the generator switches to a non-class form, minifies, or emits
// generic Partial<> wrappers in a different shape), surface the
// shape-change rather than tightening the regex into false-confidence.

import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
// frontend/src/test/ → frontend/ → bindings/dappco.re/go/models.ts
const MODELS_TS = resolve(
  __dirname,
  "../../bindings/dappco.re/go/models.ts",
);

/** Extract the `{ … }` body of `export class Core` from the source.
 *  Brace-balanced scan from the first `{` after the class header.
 *  Returns the body WITHOUT the enclosing braces. */
function extractCoreClassBody(source: string): string {
  const headerIdx = source.indexOf("export class Core");
  if (headerIdx < 0) {
    throw new Error(
      "bindings-invariants: `export class Core` not found in models.ts — " +
        "binding shape has changed; surface before tightening the test.",
    );
  }
  const openIdx = source.indexOf("{", headerIdx);
  if (openIdx < 0) {
    throw new Error(
      "bindings-invariants: `export class Core` has no `{` — binding " +
        "shape has changed; surface before tightening the test.",
    );
  }
  let depth = 0;
  for (let i = openIdx; i < source.length; i++) {
    const ch = source[i];
    if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) {
        return source.slice(openIdx + 1, i);
      }
    }
  }
  throw new Error(
    "bindings-invariants: `export class Core` body never closes — " +
      "binding shape malformed; surface before tightening the test.",
  );
}

describe("bindings-invariants — wails3-generator nested-struct shape", () => {
  const source = readFileSync(MODELS_TS, "utf8");
  const body = extractCoreClassBody(source);

  it("TestBindings_CoreClass_NoCallableMethods — Core has only constructor + static createFrom", () => {
    // Strip line comments so JSDoc-mentioned method names don't trip
    // the method-position scan (the class is preceded by a docblock
    // referencing `core.New(...)` etc; the EXTRACTED body has its own
    // inline `/** … */` blocks too).
    const stripped = body
      // block comments /** … */
      .replace(/\/\*[\s\S]*?\*\//g, "")
      // line comments // …
      .replace(/\/\/[^\n]*/g, "");

    // Method-position pattern in a TS class body:
    //   ^<whitespace><name>(<args>): <ReturnType> {
    //   ^<whitespace>static <name>(<args>): <ReturnType> {
    //   ^<whitespace>async <name>(<args>): <ReturnType> {
    //
    // Allowed: `constructor(` and `static createFrom(`. Forbidden:
    // anything else at method-position. The wails3-generator emits
    // PROPERTY declarations as `"Name": Type;` (quoted-identifier
    // colon) — that shape never matches `\w+\(`.
    const methodLineRe = /^\s*(?:static\s+|async\s+|public\s+|private\s+|protected\s+)*([A-Za-z_$][\w$]*)\s*\(/gm;

    const forbidden: string[] = [];
    for (const m of stripped.matchAll(methodLineRe)) {
      const name = m[1];
      if (name === "constructor") continue;
      if (name === "createFrom") continue;
      forbidden.push(name);
    }

    if (forbidden.length > 0) {
      throw new Error(
        "bindings-invariants: Core class has unexpected callable " +
          "method(s): " +
          JSON.stringify(forbidden) +
          "\n\nThe wails3-generator should marshal core.Core's nested " +
          "struct fields as method-less Partial<> shapes. The " +
          "presence of instance methods means the bindings boundary " +
          "is exporting RPC handles directly — the sandboxsubstrate " +
          "TierGoOnly gate (Mantis #1664 Phase B) cannot rely on the " +
          "method-less invariant. Re-inspect the binding shape and " +
          "either fix upstream wails3 or update this test deliberately.",
      );
    }
    expect(forbidden).toEqual([]);
  });

  it("TestBindings_CoreClass_HasConstructor_Sanity — body still parses as a class", () => {
    // Sanity check that the brace-balanced extractor pulled a class
    // body, not an empty string or a sibling block. Without this, a
    // future generator change that drops the Core class entirely would
    // leave the forbidden-methods scan trivially passing.
    expect(body).toMatch(/constructor\s*\(/);
    expect(body).toMatch(/static\s+createFrom\s*\(/);
  });
});
