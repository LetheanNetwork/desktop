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
});
