// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// vi.mock is hoisted above any `import` — declare every dynamic
// dependency the element resolves in connectedCallback / handlers so
// the test runs without network, FS, or live wails bindings.

vi.mock("@desktop/opencode/wailsservice", () => ({
  WStatus:                vi.fn(async () => ({ OK: true, Value: [] })),
  WStart:                 vi.fn(async () => ({ OK: true, Value: "" })),
  WStop:                  vi.fn(async () => ({ OK: true, Value: "" })),
  WMergeHostConfig:       vi.fn(async () => ({ OK: true, Value: null })),
  WOpenWebWindow:         vi.fn(async () => ({ OK: true, Value: null })),
  WOpenTUI:               vi.fn(async () => ({ OK: true, Value: null })),
  WOpenStudio:            vi.fn(async () => ({ OK: true, Value: null })),
  WIsStudioInstalled:     vi.fn(async () => ({ OK: true, Value: false })),
  WUpgrade:               vi.fn(async () => ({ OK: false, Value: { Code: "upgrade.requires_confirmation" } })),
  WUpgradeWithConsent:    vi.fn(async () => ({ OK: true, Value: { updated: false } })),
}));

vi.mock("@desktop/integrations/wailsservice", () => ({
  List: vi.fn(async () => ([
    {
      id: "opencode",
      name: "opencode",
      description: "opencode sandbox",
      config_path: "~/.config/opencode/opencode.jsonc",
      config_path_raw: "~/.config/opencode/opencode.jsonc",
      exists: true,
      state: "configured",
    },
  ])),
}));

vi.mock("@desktop/runner/service", () => ({
  WModels: vi.fn(async () => ({ OK: true, Value: ["gemma3:12b-it-q4_K_M"] })),
}));

vi.mock("@desktop/server/service", () => ({
  WAddr: vi.fn(async () => ({ OK: true, Value: ":8000" })),
}));

vi.mock("@desktop/apikey/wailsservice", () => ({
  Masked: vi.fn(async () => ({ OK: true, Value: "sk-lthn-•••• (test)" })),
}));

import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import { setCallHandler, clearCallHandlers, setDefaultCallHandler, resetDefaultCallHandler } from "../../test/setup-helpers";

/** Binding-id for i18n T() — set.up.ts's legacy allowlist baked this
 *  value in. Re-using it here so the connectedCallback Promise.all
 *  resolves with a usable string per messageID rather than null. */
const BID_I18N_T = 1099757357;
import { WUpgrade, WUpgradeWithConsent } from "@desktop/opencode/wailsservice";
import "./integrations-window";

/** Wait long enough for the lazy connectedCallback awaits (i18n + 4
 *  dynamic imports + initial pollOpencode) to settle so the opencode
 *  card actually renders and the "Check for updates" button is in
 *  the DOM. Two macrotask flips cover the chain we observe in
 *  practice. */
async function settleConnect(el: HTMLElement) {
  const lit = el as unknown as { updateComplete?: Promise<boolean> };
  for (let i = 0; i < 5; i++) {
    if (lit.updateComplete) await lit.updateComplete;
    await new Promise((r) => setTimeout(r, 0));
  }
}

beforeEach(() => {
  clearCallHandlers();
  // T() (i18n) routes through @wailsio/runtime Call.ByID. Echo the
  // message-id back so connectedCallback's Promise.all([T(...)])
  // resolves to a usable string per key. Without this, render hits
  // null.replace() inside the opencode card and panics.
  setCallHandler(BID_I18N_T, async (messageID: unknown) => String(messageID ?? ""));
  // Any other binding-id from incidental traffic resolves to null so
  // it doesn't reject and trigger Lit's unhandled rejection path.
  setDefaultCallHandler(async () => null);
  vi.mocked(WUpgrade).mockClear();
  vi.mocked(WUpgradeWithConsent).mockClear();
});

afterEach(() => {
  clearCallHandlers();
  resetDefaultCallHandler();
});

describe("lthn-integrations-window — smoke", () => {
  it("mounts with the Integrations titlebar", async () => {
    const { host } = await mountWindow("lthn-integrations-window");
    expectChromeTitle(host, "Integrations");
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-integrations-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-integrations-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-integrations-window — Upgrade dialog (Mantis #1623)", () => {
  it("Test_UpgradeButton_OpensConfirmDialog_Good — Check-for-updates click renders the supply-chain dialog", async () => {
    const { host, el } = await mountWindow("lthn-integrations-window");
    await settleConnect(el);

    // No dialog before click.
    expect(host.querySelector("[data-testid='oc-upgrade-dialog']")).toBeNull();

    const btn = host.querySelector("[data-testid='oc-check-updates']") as HTMLButtonElement | null;
    expect(btn, "opencode card must render the Check-for-updates button").not.toBeNull();
    btn!.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    const dialog = host.querySelector("[data-testid='oc-upgrade-dialog']");
    expect(dialog, "click must open the supply-chain warning dialog").not.toBeNull();
    expect(dialog!.getAttribute("role")).toBe("dialog");
    // Click alone must NOT fire either upgrade path — that only happens
    // after the user confirms inside the dialog.
    expect(WUpgrade).not.toHaveBeenCalled();
    expect(WUpgradeWithConsent).not.toHaveBeenCalled();
  });

  it("Test_UpgradeDialog_Confirm_CallsWithConsent_Good — Confirm fires WUpgradeWithConsent with consent flag", async () => {
    const { host, el } = await mountWindow("lthn-integrations-window");
    await settleConnect(el);

    (host.querySelector("[data-testid='oc-check-updates']") as HTMLButtonElement).click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    const confirm = host.querySelector("[data-testid='oc-upgrade-confirm']") as HTMLButtonElement | null;
    expect(confirm, "dialog must expose a confirm button").not.toBeNull();
    confirm!.click();
    await settleConnect(el);

    expect(WUpgradeWithConsent).toHaveBeenCalledTimes(1);
    expect(WUpgradeWithConsent).toHaveBeenCalledWith({
      confirmed_by_user: true,
      restart_sandboxes: false,
    });
    // Legacy fail-closed WUpgrade must NOT be invoked from the
    // dialog confirm path — that's the regression this lane guards.
    expect(WUpgrade).not.toHaveBeenCalled();
    // Dialog closes after confirm.
    expect(host.querySelector("[data-testid='oc-upgrade-dialog']")).toBeNull();
  });

  it("Test_UpgradeDialog_Cancel_NoCall_Good — Cancel closes dialog without calling either Upgrade path", async () => {
    const { host, el } = await mountWindow("lthn-integrations-window");
    await settleConnect(el);

    (host.querySelector("[data-testid='oc-check-updates']") as HTMLButtonElement).click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(host.querySelector("[data-testid='oc-upgrade-dialog']")).not.toBeNull();

    const cancel = host.querySelector("[data-testid='oc-upgrade-cancel']") as HTMLButtonElement | null;
    expect(cancel, "dialog must expose a cancel button").not.toBeNull();
    cancel!.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    expect(host.querySelector("[data-testid='oc-upgrade-dialog']")).toBeNull();
    expect(WUpgrade).not.toHaveBeenCalled();
    expect(WUpgradeWithConsent).not.toHaveBeenCalled();
  });
});
