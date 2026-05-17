// SPDX-Licence-Identifier: EUPL-1.2
//
// boot-probe.test.ts — Mantis #1500 fail-CLOSED unit tests.
//
// The previous in-line IIFE in main.ts silently degraded on every
// failure mode of the AccountStatus probe — binding-load failure,
// shape drift, IPC rejection, Result.OK=false — leaving the
// underlying surface (tray / chat / settings / canvas) rendering
// UNPROTECTED with no auth overlay and no console signal.
//
// These tests pin the fail-CLOSED contract. Every failure-mode
// branch MUST mount an error overlay AND emit a structured
// console.error breadcrumb. The happy path MUST stay clean (no
// mount, no log noise).
//
// Probe shape note (per the ticket's SECURITY-NOTE escape valve):
// the probe is wired through Wails IPC, NOT an HTTP fetch. The
// ticket's test-matrix names map to the Wails failure taxonomy as
// follows:
//
//   NetworkError → BindingImportRejects + CallRejects
//   401          → AccountAbsent (the unauthenticated-state signal
//                  IS has_user_account=false on Wails, not a 401)
//   5xx          → ResultOKFalse (degraded backend signal)
//   HappyPath    → AccountPresent (no mount; apiFetch interceptor
//                  handles downstream 401s)

import { describe, it, expect, beforeEach, vi } from "vitest";
import type { Mock } from "vitest";
import { runBootProbe, type MountFn, type ServerKeyImporter } from "./boot-probe";

type MountCall = Parameters<MountFn>;

interface ProbeHarness {
  mount: MountFn & Mock;
  mountCalls: MountCall[];
}

function makeHarness(): ProbeHarness {
  const calls: MountCall[] = [];
  const mount = vi.fn((...args: MountCall) => { calls.push(args); }) as unknown as MountFn & Mock;
  return { mount, mountCalls: calls };
}

/** Helper — synthesise a serverkey module whose AccountStatus returns
 *  the supplied result. The shape mirrors the auto-generated binding
 *  at frontend/bindings/.../serverkey/service.ts. */
function fakeServerKeyModule(
  result: { OK?: boolean; Value?: { has_user_account?: boolean } } | Promise<never>,
): ServerKeyImporter {
  return async () => ({
    AccountStatus: () =>
      result instanceof Promise ? result : Promise.resolve(result),
  });
}

describe("runBootProbe — Mantis #1500 fail-CLOSED contract", () => {
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    // Each test asserts on its own console.error invocations; reset
    // the spy between tests so call counts don't accumulate.
    errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  // The "network error" equivalent for a Wails-bound probe is the
  // binding module failing to load (bundler glitch, file absent from
  // the bindings/ tree after a regeneration regression). Previously
  // .catch(() => null) swallowed this — the probe just returned
  // without mounting anything.
  it("TestBootProbe_BindingImportRejects_FailsClosed_Bad", async () => {
    const { mount, mountCalls } = makeHarness();
    const importer: ServerKeyImporter = async () => {
      throw new Error("vite: cannot resolve @desktop/serverkey/service");
    };

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("unreachable");
    if (outcome.kind === "unreachable") {
      expect(outcome.reason).toMatch(/binding failed to load/);
    }
    expect(mount).toHaveBeenCalledTimes(1);
    expect(mountCalls[0][0]).toBe("error");
    expect(mountCalls[0][1]?.errorMessage).toMatch(/Cannot reach lthn server/);
    expect(mountCalls[0][1]?.errorBody).toBeTruthy();
    expect(errorSpy).toHaveBeenCalledTimes(1);
    expect(errorSpy.mock.calls[0][0]).toContain("[lthn:boot-probe]");
  });

  // Schema drift — the module loads but no longer exports
  // AccountStatus (Wails3 generate rename / removal). Previously the
  // `if (!svc || typeof svc.AccountStatus !== 'function') return`
  // exit silently treated this as "no probe needed".
  it("TestBootProbe_BindingShapeWrong_FailsClosed_Bad", async () => {
    const { mount, mountCalls } = makeHarness();
    const importer: ServerKeyImporter = async () => ({ /* no AccountStatus */ });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("unreachable");
    if (outcome.kind === "unreachable") {
      expect(outcome.reason).toMatch(/missing AccountStatus/);
    }
    expect(mount).toHaveBeenCalledTimes(1);
    expect(mountCalls[0][0]).toBe("error");
    expect(errorSpy).toHaveBeenCalledTimes(1);
  });

  // The Wails IPC call itself rejects — transport-layer failure
  // (runtime not initialised, Go panic mid-call, websocket dropped).
  // Previously the outer try/catch swallowed this with a comment
  // claiming "binding missing → degrade silently".
  it("TestBootProbe_CallRejects_FailsClosed_Bad", async () => {
    const { mount, mountCalls } = makeHarness();
    const importer: ServerKeyImporter = async () => ({
      AccountStatus: () => Promise.reject(new Error("wails: transport closed")),
    });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("unreachable");
    if (outcome.kind === "unreachable") {
      expect(outcome.reason).toMatch(/call rejected/);
    }
    expect(mount).toHaveBeenCalledTimes(1);
    expect(mountCalls[0][0]).toBe("error");
    expect(mountCalls[0][1]?.errorMessage).toMatch(/Cannot reach lthn server/);
    expect(errorSpy).toHaveBeenCalledTimes(1);
  });

  // The "5xx" equivalent for the Wails probe is OK=false on the
  // Result envelope — the Go side ran but reports a degraded backend
  // (store error, account.Service unavailable). Previously the
  // condition `r?.OK !== false && r?.Value?.has_user_account === false`
  // silently treated OK=false as "fine, no setup needed" because the
  // outer `===` lhs was the only mount trigger.
  it("TestBootProbe_ResultOKFalse_FailsClosed_Bad", async () => {
    const { mount, mountCalls } = makeHarness();
    const importer = fakeServerKeyModule({ OK: false });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("unreachable");
    if (outcome.kind === "unreachable") {
      expect(outcome.reason).toMatch(/degraded result/);
    }
    expect(mount).toHaveBeenCalledTimes(1);
    expect(mountCalls[0][0]).toBe("error");
    expect(errorSpy).toHaveBeenCalledTimes(1);
  });

  // The "expected unauthenticated state" — no account on disk yet.
  // The probe SHOULD mount setup-gate here (this is the first-run
  // bootstrap path). Equivalent of the ticket's
  // 401_RendersAuthGate_Good case for the HTTP shape.
  it("TestBootProbe_AccountAbsent_RendersSetupGate_Good", async () => {
    const { mount, mountCalls } = makeHarness();
    const importer = fakeServerKeyModule({
      OK: true,
      Value: { has_user_account: false },
    });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("ok-setup");
    expect(mount).toHaveBeenCalledTimes(1);
    expect(mountCalls[0][0]).toBe("setup");
    expect(mountCalls[0][1]).toBeUndefined();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  // Happy path — account exists on disk. The probe must NOT mount;
  // the apiFetch 401 interceptor handles unauthenticated calls
  // downstream by dispatching lthn:auth:401, which the main.ts
  // window listener turns into mount("error", reqId).
  it("TestBootProbe_AccountPresent_NoMount_Good", async () => {
    const { mount } = makeHarness();
    const importer = fakeServerKeyModule({
      OK: true,
      Value: { has_user_account: true },
    });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("ok-account");
    expect(mount).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  // Defensive — Value present but has_user_account unspecified. The
  // probe defaults to do-not-mount because the downstream apiFetch
  // 401 path still catches the unauthenticated case. Asserts the
  // fail-CLOSED bar applies only to UNREACHABLE outcomes, not to
  // ambiguous-but-present ones.
  it("TestBootProbe_HasUserAccountAbsentField_NoMount_Good", async () => {
    const { mount } = makeHarness();
    const importer = fakeServerKeyModule({ OK: true, Value: {} });

    const outcome = await runBootProbe(mount, importer);

    expect(outcome.kind).toBe("ok-account");
    expect(mount).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });
});
