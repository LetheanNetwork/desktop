// SPDX-Licence-Identifier: EUPL-1.2

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CONFLICT_409_EVENT,
  ConflictDetail,
  handleStaleVersionConflict,
} from "./conflict-dispatch";

describe("conflict-dispatch · handleStaleVersionConflict", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns false for an OK core.Result (no event dispatched)", () => {
    const spy = vi.spyOn(window, "dispatchEvent");
    const handled = handleStaleVersionConflict(
      { OK: true, Value: { something: "ok" } },
      "sales.contacts.update",
    );
    expect(handled).toBe(false);
    expect(spy).not.toHaveBeenCalled();
  });

  it("returns true + dispatches CONFLICT_409_EVENT for direct-shape envelope", () => {
    let observed: ConflictDetail | null = null;
    const listener = (e: Event) => {
      observed = (e as CustomEvent<ConflictDetail>).detail;
    };
    window.addEventListener(CONFLICT_409_EVENT, listener);
    try {
      const handled = handleStaleVersionConflict(
        {
          OK: false,
          Value: {
            code: "sales.contacts.update.conflict",
            current_version: 7,
            current_hash: "deadbeef",
          },
        },
        "sales.contacts.update",
      );
      expect(handled).toBe(true);
      expect(observed).not.toBeNull();
      expect(observed!.currentVersion).toBe(7);
      expect(observed!.currentHash).toBe("deadbeef");
      expect(observed!.errorCode).toBe("sales.contacts.update.conflict");
      expect(observed!.service).toBe("sales.contacts.update");
    } finally {
      window.removeEventListener(CONFLICT_409_EVENT, listener);
    }
  });

  it("returns true + dispatches for nested error.code shape", () => {
    let observed: ConflictDetail | null = null;
    const listener = (e: Event) => {
      observed = (e as CustomEvent<ConflictDetail>).detail;
    };
    window.addEventListener(CONFLICT_409_EVENT, listener);
    try {
      const handled = handleStaleVersionConflict(
        {
          OK: false,
          Value: {
            error: {
              code: "sales.deals.update.conflict",
              current_version: 4,
              current_hash: "abc123",
            },
          },
        },
        "sales.deals.update",
      );
      expect(handled).toBe(true);
      expect(observed).not.toBeNull();
      expect(observed!.errorCode).toBe("sales.deals.update.conflict");
      expect(observed!.service).toBe("sales.deals.update");
    } finally {
      window.removeEventListener(CONFLICT_409_EVENT, listener);
    }
  });

  it("returns false for unrelated errors (no .conflict suffix)", () => {
    const spy = vi.spyOn(window, "dispatchEvent");
    const handled = handleStaleVersionConflict(
      {
        OK: false,
        Value: {
          code: "sales.contacts.update.permission_denied",
          message: "no",
        },
      },
      "sales.contacts.update",
    );
    expect(handled).toBe(false);
    expect(spy).not.toHaveBeenCalled();
  });

  it("event detail carries service identifier the call-site supplied", () => {
    let observed: ConflictDetail | null = null;
    const listener = (e: Event) => {
      observed = (e as CustomEvent<ConflictDetail>).detail;
    };
    window.addEventListener(CONFLICT_409_EVENT, listener);
    try {
      handleStaleVersionConflict(
        {
          OK: false,
          Value: { code: "sales.pipeline.update.conflict", current_version: 1 },
        },
        "sales.pipeline.update",
      );
      expect(observed!.service).toBe("sales.pipeline.update");
    } finally {
      window.removeEventListener(CONFLICT_409_EVENT, listener);
    }
  });

  // Mantis #1544 / #1547 — pin the wire shape produced by Go-side
  // core.Fail(paths.ConflictEnvelope{...}). The fixture below is the
  // EXACT JSON shape that lands on Result.Value after Wails marshals a
  // paths.ConflictEnvelope through encoding/json — lowercase
  // snake_case keys per the json tags on the ConflictEnvelope struct.
  //
  // The PRIOR shape (now-banned) — a *core.Err wrap — marshalled as
  // {"Operation":"contacts.atomicWriteFile",
  //  "Message":"contacts.update.conflict",
  //  "Cause":{}, "Code":""} — PascalCase keys, empty Code, message-
  // carries-the-code. extractEnvelope returned null on that shape.
  // Asserting both fixtures here prevents the regression from
  // recurring (cascade W2/W3/W4 inherit this pattern).
  describe("real Wails-marshalled shape (Mantis #1544/#1547)", () => {
    // typeof JSON.stringify(paths.ConflictEnvelope{...}) — captured
    // from go/pkg/sales/contacts/contacts_test.go round-trip.
    const realConflictValueJSON = JSON.stringify({
      code: "contacts.update.conflict",
      current_version: 7,
      current_hash: "8f9e2c1a4b6d5e3f",
    });

    it("dispatches on real ConflictEnvelope marshalled shape (direct on Value)", () => {
      let observed: ConflictDetail | null = null;
      const listener = (e: Event) => {
        observed = (e as CustomEvent<ConflictDetail>).detail;
      };
      window.addEventListener(CONFLICT_409_EVENT, listener);
      try {
        // Parse the JSON the way the Wails RPC layer would hand it to
        // a frontend caller — Result is {OK: false, Value: <parsed>}.
        const parsed = JSON.parse(realConflictValueJSON);
        const result = { OK: false, Value: parsed };
        const handled = handleStaleVersionConflict(
          result,
          "sales.contacts.update",
        );
        expect(handled).toBe(true);
        expect(observed).not.toBeNull();
        expect(observed!.errorCode).toBe("contacts.update.conflict");
        expect(observed!.currentVersion).toBe(7);
        expect(observed!.currentHash).toBe("8f9e2c1a4b6d5e3f");
        expect(observed!.service).toBe("sales.contacts.update");
      } finally {
        window.removeEventListener(CONFLICT_409_EVENT, listener);
      }
    });

    it("rejects the legacy *core.Err PascalCase shape (the #1544 regression fixture)", () => {
      // Pre-fix marshalled shape — PascalCase, empty Code, message
      // carries the code. extractEnvelope MUST return null so the
      // helper reports false and the caller falls through to its
      // generic error UI rather than rendering an empty toast.
      const legacyValueJSON = JSON.stringify({
        Operation: "contacts.atomicWriteFile",
        Message: "contacts.update.conflict",
        Cause: {},
        Code: "",
      });
      const parsed = JSON.parse(legacyValueJSON);
      const result = { OK: false, Value: parsed };
      const spy = vi.spyOn(window, "dispatchEvent");
      const handled = handleStaleVersionConflict(
        result,
        "sales.contacts.update",
      );
      expect(handled).toBe(false);
      expect(spy).not.toHaveBeenCalled();
    });
  });
});
