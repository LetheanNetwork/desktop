// SPDX-Licence-Identifier: EUPL-1.2
//
// Coverage for <lthn-marketplace-window> (Mantis #1628).
//
// Mirrors the sibling-ext test patterns:
//   - tools-window.test.ts / fleet-window.test.ts — smoke + two-shell shape
//   - integrations-window.test.ts — vi.mock @desktop/<pkg>/service module
//     at suite scope + setCallHandler echo for i18n T().
//
// Mocks the @desktop/marketplace/service module so the catalogue helper
// (_marketplace-catalogue.ts ⇒ FetchIndex) and the window-direct calls
// (Install / Launch / Stop / Uninstall / ListInstalled / FetchManifest)
// route through the same vi.fn surface. Each spec installs its own
// fixtures via vi.mocked(Fn).mockResolvedValue(...) per the canonical
// lit-view-backend-wire pattern.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// vi.mock is hoisted above any `import` — declare the marketplace service
// surface so the connectedCallback _refresh + later Install path resolve
// without a real Wails bridge.
vi.mock("@desktop/marketplace/service", () => ({
  FetchIndex:     vi.fn(async () => ({ OK: true, Value: { Entries: [], CachedAt: "", FromCache: false } })),
  FetchManifest:  vi.fn(async () => ({ OK: true, Value: {} })),
  Install:        vi.fn(async () => ({ OK: true, Value: null })),
  Launch:         vi.fn(async () => ({ OK: true, Value: null })),
  Stop:           vi.fn(async () => ({ OK: true, Value: null })),
  Uninstall:      vi.fn(async () => ({ OK: true, Value: null })),
  ListInstalled:  vi.fn(async () => ({ OK: true, Value: [] })),
  Status:         vi.fn(async () => ({ OK: true, Value: null })),
}));

import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import { setCallHandler, clearCallHandlers, setDefaultCallHandler, resetDefaultCallHandler } from "../../test/setup-helpers";
import {
  FetchIndex, FetchManifest, Install, Uninstall, ListInstalled,
} from "@desktop/marketplace/service";
import "./marketplace-window";

/** Binding-id for i18n T() — set in setup.ts's legacy allowlist. Echoing
 *  the message-id back is enough for the chrome / labels to resolve to
 *  a usable string per key. Without this, the connectedCallback
 *  Promise.all([T(title), T(subtitle)]) rejects and _refresh never runs.
 */
const BID_I18N_T = 1099757357;

/** Wait long enough for the lazy connectedCallback chain to settle:
 *   - Promise.all([T(title), T(subtitle)]) → chrome assignment
 *   - _loadT() async i18n batch
 *   - _refresh → fetchCatalogue + ListInstalled
 *   - dynamic import("@wailsio/runtime") for Events.On
 *  Three macrotask flips reliably cover the chain we observe. */
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
  // resolves to a usable string per key.
  setCallHandler(BID_I18N_T, async (messageID: unknown) => String(messageID ?? ""));
  // Any incidental binding-id traffic resolves to null so it doesn't
  // reject and trigger Lit's unhandled-rejection path.
  setDefaultCallHandler(async () => null);
  // Reset module-scope vi.fn defaults to the empty-catalogue baseline so
  // each spec starts from a known state. Specs that want a populated
  // catalogue / installed list reassign before mountWindow.
  vi.mocked(FetchIndex).mockReset();
  vi.mocked(FetchManifest).mockReset();
  vi.mocked(Install).mockReset();
  vi.mocked(Uninstall).mockReset();
  vi.mocked(ListInstalled).mockReset();
  vi.mocked(FetchIndex).mockResolvedValue({ OK: true, Value: { Entries: [], CachedAt: "", FromCache: false } });
  vi.mocked(FetchManifest).mockResolvedValue({ OK: true, Value: {} });
  vi.mocked(Install).mockResolvedValue({ OK: true, Value: null });
  vi.mocked(Uninstall).mockResolvedValue({ OK: true, Value: null });
  vi.mocked(ListInstalled).mockResolvedValue({ OK: true, Value: [] });
});

afterEach(() => {
  clearCallHandlers();
  resetDefaultCallHandler();
});

describe("lthn-marketplace-window — smoke", () => {
  it("mounts with the Marketplace titlebar", async () => {
    const { host } = await mountWindow("lthn-marketplace-window");
    expectChromeTitle(host, "Marketplace");
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-marketplace-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-marketplace-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-marketplace-window — Browse tab catalogue (Mantis #1628 §1)", () => {
  it("renders catalogue cards from mocked FetchIndex", async () => {
    vi.mocked(FetchIndex).mockResolvedValue({
      OK: true,
      Value: {
        Entries: [{
          name:            "lthn-dev",
          display:         "Lethean Dev",
          description:     "Test catalogue entry for unit tests.",
          category:        "devtools",
          icon:            "fa-code",
          homepage:        "",
          license:         "EUPL-1.2",
          sourceURL:       "https://example.test/lthn-dev/manifest.yaml",
          requiresVersion: "0.1.0",
          lastUpdated:     "",
        }],
        CachedAt:  "",
        FromCache: false,
      },
    });

    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    expect(vi.mocked(FetchIndex)).toHaveBeenCalled();
    // Light DOM (createRenderRoot returns this) — query host directly.
    const text = host.textContent ?? "";
    expect(text).toContain("Lethean Dev");
    expect(text).toContain("lthn-dev");
  });

  it("empty catalogue keeps the empty-state copy (rows-replace baseline)", async () => {
    // Default empty mock from beforeEach stands.
    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    expect(vi.mocked(FetchIndex)).toHaveBeenCalled();
    // 0 catalogue cards rendered.
    expect(host.querySelector("p")).toBeNull();
  });

  it("FetchIndex rejection keeps the catalogue empty (reject-keeps fallback)", async () => {
    vi.mocked(FetchIndex).mockRejectedValue(new Error("network unreachable"));
    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    // fetchCatalogue swallows the error and returns an empty index so
    // the window still renders the empty-state rather than crashing.
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-marketplace-window — Install flow (Mantis #1628 §2, §4)", () => {
  it("first install click opens the permission-disclosure modal before Install fires", async () => {
    vi.mocked(FetchIndex).mockResolvedValue({
      OK: true,
      Value: {
        Entries: [{
          name:            "lthn-dev",
          display:         "Lethean Dev",
          description:     "",
          category:        "devtools",
          icon:            "fa-code",
          homepage:        "",
          license:         "EUPL-1.2",
          sourceURL:       "https://example.test/lthn-dev/manifest.yaml",
          requiresVersion: "0.1.0",
          lastUpdated:     "",
        }],
        CachedAt:  "",
        FromCache: false,
      },
    });
    // Manifest declares one permission — the install flow MUST stop on
    // the perms step before calling Install, per §4 done-criteria.
    vi.mocked(FetchManifest).mockResolvedValue({
      OK: true,
      Value: {
        Env:         [],
        Permissions: [{ Scope: "fs/read", Mode: "read", Reason: "Read project files" }],
      },
    });

    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    // Find the Install button — buttons render as <lthn-btn> custom
    // elements. After _loadT() resolves the i18n stub, button text is
    // the message-id "btn_install" (echoed by setCallHandler), so match
    // either the constructor default or the i18n echo.
    const installBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => /Install|btn_install/.test(b.textContent ?? ""));
    expect(installBtn, "Browse pane must render an Install button on each card").toBeDefined();
    (installBtn as HTMLElement).click();
    await settleConnect(el);

    // FetchManifest fired, Install did NOT.
    expect(vi.mocked(FetchManifest)).toHaveBeenCalledWith("https://example.test/lthn-dev/manifest.yaml");
    expect(vi.mocked(Install)).not.toHaveBeenCalled();

    // The perms modal must now be present — copy comes from the i18n
    // stub echoing the message-id back, so we look for the bundle
    // display name which is interpolated as the modal heading prefix.
    const text = host.textContent ?? "";
    expect(text).toContain("Lethean Dev");
    // Approve button text is the i18n stub of perm_approve.
    const approveBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => (b.textContent ?? "").includes("perm_approve"));
    expect(approveBtn, "perms modal must expose an approve button").toBeDefined();
  });

  it("approve in perms modal calls Install with the bundle manifest payload", async () => {
    vi.mocked(FetchIndex).mockResolvedValue({
      OK: true,
      Value: {
        Entries: [{
          name:            "lthn-dev",
          display:         "Lethean Dev",
          description:     "",
          category:        "devtools",
          icon:            "fa-code",
          homepage:        "",
          license:         "EUPL-1.2",
          sourceURL:       "https://example.test/lthn-dev/manifest.yaml",
          requiresVersion: "0.1.0",
          lastUpdated:     "",
        }],
        CachedAt: "", FromCache: false,
      },
    });
    const manifestFixture = {
      Env:         [],
      Permissions: [{ Scope: "fs/read", Mode: "read", Reason: "Read project files" }],
    };
    vi.mocked(FetchManifest).mockResolvedValue({ OK: true, Value: manifestFixture });

    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    const installBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => /Install|btn_install/.test(b.textContent ?? ""));
    expect(installBtn, "Browse pane must render an Install button").toBeDefined();
    (installBtn as HTMLElement).click();
    await settleConnect(el);

    const approveBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => (b.textContent ?? "").includes("perm_approve"));
    expect(approveBtn, "approve button must be present after perms modal opens").toBeDefined();
    (approveBtn as HTMLElement).click();
    await settleConnect(el);

    expect(vi.mocked(Install)).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(Install).mock.calls[0][0] as { Manifest: unknown; Env: Record<string, string> };
    expect(arg.Manifest).toEqual(manifestFixture);
    // No env declared on the fixture manifest, so Env is the empty map.
    expect(arg.Env).toEqual({});
  });

  it("manifest with secret env entry opens the env-prompt modal before perms", async () => {
    vi.mocked(FetchIndex).mockResolvedValue({
      OK: true,
      Value: {
        Entries: [{
          name:            "lthn-dev",
          display:         "Lethean Dev",
          description:     "",
          category:        "devtools",
          icon:            "fa-code",
          homepage:        "",
          license:         "EUPL-1.2",
          sourceURL:       "https://example.test/lthn-dev/manifest.yaml",
          requiresVersion: "0.1.0",
          lastUpdated:     "",
        }],
        CachedAt: "", FromCache: false,
      },
    });
    // The component opens the env step only when a secret has no
    // default value (Default is empty or starts with ${random}). A
    // plain text field with no default does NOT trigger env step —
    // it goes straight to perms or installs immediately.
    vi.mocked(FetchManifest).mockResolvedValue({
      OK: true,
      Value: {
        Env: [{ Key: "API_KEY", Prompt: "API key", Type: "secret", Default: "" }],
        Permissions: [],
      },
    });

    const { host, el } = await mountWindow("lthn-marketplace-window");
    await settleConnect(el);

    const installBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => /Install|btn_install/.test(b.textContent ?? ""));
    expect(installBtn, "Browse pane must render an Install button").toBeDefined();
    (installBtn as HTMLElement).click();
    await settleConnect(el);

    expect(vi.mocked(Install)).not.toHaveBeenCalled();
    // env-prompt modal — Continue button uses i18n stub of env_next.
    const nextBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => (b.textContent ?? "").includes("env_next"));
    expect(nextBtn, "env-prompt modal must expose the Continue button").toBeDefined();
    // Password input rendered for the secret-typed field.
    const pwd = host.querySelector('input[type="password"]') as HTMLInputElement | null;
    expect(pwd, "secret env entry must render as a password input").not.toBeNull();
  });
});

describe("lthn-marketplace-window — Installed pane + lifecycle event (Mantis #1628 §3, §6)", () => {
  it("status pill updates when marketplace:bundle:changed delivers a launched payload", async () => {
    vi.mocked(ListInstalled).mockResolvedValue({
      OK: true,
      Value: [{
        BundleID:       "lthn-dev",
        Display:        "Lethean Dev",
        ManifestSchema: "v1",
        Status:         "stopped",
        InstalledAt:    "",
      }],
    });

    const { host, el } = await mountWindow("lthn-marketplace-window", {
      props: { tab: "installed" },
    });
    await settleConnect(el);

    // Status pill renders the i18n stub of status_stopped.
    let text = host.textContent ?? "";
    expect(text).toContain("status_stopped");

    // Drive the event handler directly. The component subscribes via
    // @wailsio/runtime Events.On in connectedCallback — the test mock
    // returns a no-op unsub, so dispatching against the live ev bus
    // wouldn't reach the handler. Reach into the instance and apply
    // the change-payload through the private method whose contract is
    // the exact shape the bridge forwards.
    (el as unknown as {
      _applyBundleChanged: (ev: {
        phase: string;
        bundle: { BundleID: string; Display: string; ManifestSchema: string; Status: string; InstalledAt: string };
        before: unknown;
        at: string;
      }) => void;
    })._applyBundleChanged({
      phase:  "launched",
      bundle: { BundleID: "lthn-dev", Display: "Lethean Dev", ManifestSchema: "v1", Status: "running", InstalledAt: "" },
      before: null,
      at:     "",
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    text = host.textContent ?? "";
    expect(text).toContain("status_running");
    expect(text).not.toContain("status_stopped");
  });

  it("Uninstall on an installed bundle calls Uninstall with the bundle id", async () => {
    vi.mocked(ListInstalled).mockResolvedValue({
      OK: true,
      Value: [{
        BundleID:       "lthn-dev",
        Display:        "Lethean Dev",
        ManifestSchema: "v1",
        Status:         "running",
        InstalledAt:    "",
      }],
    });

    const { host, el } = await mountWindow("lthn-marketplace-window", {
      props: { tab: "installed" },
    });
    await settleConnect(el);

    // Find the Uninstall button — i18n stub renders "btn_uninstall".
    const uninstallBtn = Array.from(host.querySelectorAll("lthn-btn"))
      .find(b => (b.textContent ?? "").includes("btn_uninstall"));
    expect(uninstallBtn, "Installed pane must render an Uninstall button").toBeDefined();
    (uninstallBtn as HTMLElement).click();
    await settleConnect(el);

    expect(vi.mocked(Uninstall)).toHaveBeenCalledWith("lthn-dev");
  });
});
