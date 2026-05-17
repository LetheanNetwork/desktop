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
import { readFileSync, readdirSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve, relative } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
// frontend/src/test/ → frontend/ → bindings/dappco.re/go/models.ts
const MODELS_TS = resolve(
  __dirname,
  "../../bindings/dappco.re/go/models.ts",
);
// frontend/src/test/ → frontend/ → bindings/ (the auto-discovery root for
// the Mantis #1694 ADD-1 nested-struct-return sweep below).
const BINDINGS_ROOT = resolve(__dirname, "../../bindings");

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

// Mantis #1694 (ADD-1) — generalise the Core-class invariant across
// EVERY generated models.ts under frontend/bindings/**. The wails3
// binding-generator's policy of marshalling nested struct returns as
// method-less classes (constructor + static createFrom only) is the
// load-bearing property the TierGoOnly substrate gate relies on
// (Cerberus #55 ADD-1 / RFC.wails-surface §6). If a future wails3
// release flips that policy — even on a sibling-pkg nested struct that
// the Sandbox-specific test never sees — the invariant silently breaks
// for that pkg and the substrate-tier assumption fragments.
//
// Auto-discover all models.ts under bindings/ and walk every
// `export class <Name>` block. Same brace-balanced extractor + same
// method-position scan as the Core-class pin above. Allow-list stays
// `constructor` + `static createFrom`.
//
// SECURITY-NOTE escape valve: if the binding shape changes in a way
// that makes string-matching brittle (generator switches to a non-class
// form, minifies, emits decorators, etc.), surface the shape-change
// rather than tightening the regex into false-confidence.

function discoverModelsFiles(root: string): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    let entries: string[];
    try {
      entries = readdirSync(dir);
    } catch {
      return;
    }
    for (const name of entries) {
      const p = resolve(dir, name);
      let st;
      try {
        st = statSync(p);
      } catch {
        continue;
      }
      if (st.isDirectory()) {
        walk(p);
      } else if (st.isFile() && name === "models.ts") {
        out.push(p);
      }
    }
  };
  walk(root);
  return out.sort();
}

// extractAllClassBodies walks the source and returns
// [{name, body}, ...] for every `export class <Name>` block, body
// WITHOUT the enclosing braces. Brace-balanced — string literals and
// template strings are NOT specially parsed; the generator output is
// regular enough that this holds (no `{` or `}` inside JSDoc / type
// annotations at class-body depth). If a future shape breaks the scan,
// the sanity assertion at the end of the test catches the regression.
function extractAllClassBodies(source: string): Array<{ name: string; body: string }> {
  const out: Array<{ name: string; body: string }> = [];
  const headerRe = /export\s+class\s+([A-Za-z_$][\w$]*)\b/g;
  for (const m of source.matchAll(headerRe)) {
    const name = m[1];
    const headerIdx = m.index ?? -1;
    if (headerIdx < 0) continue;
    const openIdx = source.indexOf("{", headerIdx);
    if (openIdx < 0) continue;
    let depth = 0;
    let bodyEnd = -1;
    for (let i = openIdx; i < source.length; i++) {
      const ch = source[i];
      if (ch === "{") depth++;
      else if (ch === "}") {
        depth--;
        if (depth === 0) {
          bodyEnd = i;
          break;
        }
      }
    }
    if (bodyEnd < 0) continue;
    out.push({ name, body: source.slice(openIdx + 1, bodyEnd) });
  }
  return out;
}

describe("bindings-invariants — all nested-struct return classes (Mantis #1694)", () => {
  const modelsFiles = discoverModelsFiles(BINDINGS_ROOT);

  it("TestBindings_AllClasses_NoCallableMethods — every generated class has only constructor + static createFrom", () => {
    // Bootstrap sanity — the sweep is load-bearing; if discovery returns
    // zero files something is wrong with the bindings layout and the
    // test would trivially pass.
    expect(modelsFiles.length).toBeGreaterThan(0);

    type Violation = { file: string; class: string; methods: string[] };
    const violations: Violation[] = [];
    let classCount = 0;

    // JS reserved words / control-flow keywords that can legally appear
    // at line-start followed by `(` inside a constructor body
    // (`if (…)`, `for (…)`, `switch (…)` etc). Method declarations
    // cannot use these names, so excluding them eliminates false
    // positives without weakening the invariant.
    const CONTROL_FLOW = new Set<string>([
      "if",
      "else",
      "for",
      "while",
      "do",
      "switch",
      "case",
      "catch",
      "return",
      "throw",
      "new",
      "typeof",
      "delete",
      "void",
      "yield",
      "await",
      "in",
      "of",
    ]);

    for (const file of modelsFiles) {
      const source = readFileSync(file, "utf8");
      const classes = extractAllClassBodies(source);
      classCount += classes.length;
      for (const { name, body } of classes) {
        const stripped = body
          // block comments /** … */
          .replace(/\/\*[\s\S]*?\*\//g, "")
          // line comments // …
          .replace(/\/\/[^\n]*/g, "");

        // Method-position scan that excludes nested-block content:
        // a method declaration appears at depth-1 inside the class
        // body (depth-0 is the class-body extracted above; nested
        // method/constructor bodies open additional braces). Walk the
        // stripped source tracking brace depth and only test
        // line-prefix matches when depth === 0.
        const forbidden: string[] = [];
        let depth = 0;
        const lines = stripped.split("\n");
        for (const line of lines) {
          // Test method-position pattern BEFORE updating depth so that
          // the line `<name>(...) {` (which opens its own brace) is
          // checked at depth-0 (its declaration line).
          if (depth === 0) {
            const m = /^\s*(?:static\s+|async\s+|public\s+|private\s+|protected\s+|get\s+|set\s+)*([A-Za-z_$][\w$]*)\s*\(/.exec(line);
            if (m) {
              const n = m[1];
              if (n !== "constructor" && n !== "createFrom" && !CONTROL_FLOW.has(n)) {
                forbidden.push(n);
              }
            }
          }
          // Update depth based on net brace count on this line.
          for (let i = 0; i < line.length; i++) {
            const ch = line[i];
            if (ch === "{") depth++;
            else if (ch === "}") depth--;
          }
        }
        if (forbidden.length > 0) {
          violations.push({
            file: relative(BINDINGS_ROOT, file),
            class: name,
            methods: forbidden,
          });
        }
      }
    }

    // Bootstrap sanity — at least one class must have been scanned;
    // an empty sweep would trivially pass.
    expect(classCount).toBeGreaterThan(0);

    if (violations.length > 0) {
      const detail = violations
        .map((v) => `  ${v.file} :: ${v.class} → ${JSON.stringify(v.methods)}`)
        .join("\n");
      throw new Error(
        "bindings-invariants: generated class(es) have unexpected " +
          "callable method(s):\n" +
          detail +
          "\n\nThe wails3-generator should marshal nested struct " +
          "returns as method-less classes (constructor + static " +
          "createFrom only). The presence of instance methods on a " +
          "generated class means the bindings boundary is exporting " +
          "RPC handles directly — the TierGoOnly substrate gate " +
          "(Mantis #1664 Phase B / RFC.wails-surface §6) cannot rely " +
          "on the method-less invariant across the affected pkg. " +
          "Re-inspect the binding shape and either fix upstream " +
          "wails3 or update this test deliberately.",
      );
    }
    expect(violations).toEqual([]);
  });
});
