// SPDX-Licence-Identifier: EUPL-1.2
//
// Golden-list parity test per RFC.stage-e-audit-viewer v2 §5.6 / §8.2
// (Cerberus #29 Q6 AFFIRMED option-d).
//
// The TS arrays in audit-constants.ts are hand-maintained mirrors of
// the Go const blocks in go/pkg/audit/types.go. This test greps the
// authoritative Go source and asserts set-equality. Drift triggers a
// loud failure with a remediation prompt rather than silently leaving
// chip-filter rows out of date.

import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { AUDIT_EVENT_NAMES, AUDIT_OUTCOMES, AUDIT_META_KEYS } from "./audit-constants";

const __dirname = dirname(fileURLToPath(import.meta.url));
// frontend/src/lit/obs/ → repo root → go/pkg/audit/types.go.
const TYPES_GO = resolve(__dirname, "../../../../go/pkg/audit/types.go");

/** Pull every string literal assigned to an identifier starting with
 *  `prefix` at top-of-line in the Go source. Walks the file line-by-
 *  line — avoids the matchAll(`const\\s*\\(([\\s\\S]*?)\\)`)
 *  failure mode where a non-greedy regex stops at the first `)` it
 *  finds (the audit/types.go file ships multiple const blocks). */
function literalsForPrefix(source: string, prefix: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const lineRe = new RegExp(`^\\s*(${prefix}\\w+)\\s*(?:[A-Za-z][\\w.]*\\s*)?=\\s*"([^"]+)"`);
  for (const raw of source.split("\n")) {
    const lm = raw.match(lineRe);
    if (lm && !seen.has(lm[2])) {
      seen.add(lm[2]);
      out.push(lm[2]);
    }
  }
  return out;
}

describe("audit-constants — golden-list parity with go/pkg/audit/types.go", () => {
  const source = readFileSync(TYPES_GO, "utf8");

  it("TestAuditConstants_EventNames_Match_Good — event-name literals mirror Go const block", () => {
    const goEventNames = literalsForPrefix(source, "Event");
    const tsEventNames = [...AUDIT_EVENT_NAMES];
    expect(new Set(tsEventNames)).toEqual(new Set(goEventNames));
    // Length-equality catches "TS array has an extra literal Go dropped".
    expect(tsEventNames.length).toBe(goEventNames.length);
    // Diagnostic: if the assertion above fires, emit the remediation
    // prompt verbatim per RFC §5.6.
    if (new Set(tsEventNames).size !== new Set(goEventNames).size) {
      throw new Error(
        "audit constants drifted — re-mirror from Go and update this golden.\n" +
        `  Go events:  ${JSON.stringify(goEventNames)}\n` +
        `  TS events:  ${JSON.stringify(tsEventNames)}`,
      );
    }
  });

  it("TestAuditConstants_Outcomes_Match_Good — outcome literals mirror Go const block", () => {
    const goOutcomes = literalsForPrefix(source, "Outcome");
    const tsOutcomes = [...AUDIT_OUTCOMES];
    expect(new Set(tsOutcomes)).toEqual(new Set(goOutcomes));
    expect(tsOutcomes.length).toBe(goOutcomes.length);
  });

  it("TestAuditConstants_MetaKeys_Match_Good — Meta-key literals mirror Go MetaKey* const block (Mantis #1720 v2)", () => {
    const goMetaKeys = literalsForPrefix(source, "MetaKey");
    const tsMetaKeys = [...AUDIT_META_KEYS];
    expect(new Set(tsMetaKeys)).toEqual(new Set(goMetaKeys));
    expect(tsMetaKeys.length).toBe(goMetaKeys.length);
  });

  it("TestAuditConstants_GoldenListMirrorsBackend_Good — combined parity assertion across all sets", () => {
    // Single-call combined assertion for the brief's named test. Same
    // discipline as the three specs above, surfaced under the canonical
    // name from the dispatch note so a failure shows the operator the
    // top-line invariant violated. Mantis #1720 v2 — MetaKey set folded
    // alongside Event / Outcome so the dual-key shape stays in lockstep
    // across the Go ↔ TS mirror.
    const goEventNames = new Set(literalsForPrefix(source, "Event"));
    const goOutcomes = new Set(literalsForPrefix(source, "Outcome"));
    const goMetaKeys = new Set(literalsForPrefix(source, "MetaKey"));
    expect(new Set(AUDIT_EVENT_NAMES)).toEqual(goEventNames);
    expect(new Set(AUDIT_OUTCOMES)).toEqual(goOutcomes);
    expect(new Set(AUDIT_META_KEYS)).toEqual(goMetaKeys);
  });
});
