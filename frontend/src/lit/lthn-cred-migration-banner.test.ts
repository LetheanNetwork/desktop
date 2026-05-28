// SPDX-Licence-Identifier: EUPL-1.2
//
// Regression lock for the snake_case wire-shape contract of
// <lthn-cred-migration-banner>.
//
// The Go MigrationStatus (pkg/runner/types.go) carries explicit
// snake_case json tags, so the WebView receives
// {pending_migration_count, plaintext_on_disk, locked_routes}. An
// earlier hand-rolled TS interface read PascalCase — so every field
// resolved to undefined, `undefined <= 0` slipped the render guard,
// and `this._status.LockedRoutes.join(...)` threw on EVERY render.
// That crash was invisible to the test suite and only surfaced via the
// live bridge error buffer. This is the test that was missing: it
// drives the real render path with the exact snake_case payload Go
// emits, so re-introducing PascalCase (or dropping the locked_routes
// guard) fails here instead of in production.
//
// _refresh is stubbed on the prototype so connectedCallback's async
// backend fetch can't race the explicit _status assignment.

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { mountWindow } from "../test/window-fixture";
import { LthnCredMigrationBanner } from "./lthn-cred-migration-banner";

interface BannerEl extends HTMLElement {
  _status:
    | {
        pending_migration_count: number;
        plaintext_on_disk: boolean;
        locked_routes?: string[];
      }
    | null;
  updateComplete: Promise<boolean>;
}

type RefreshProto = { _refresh: () => Promise<void> };

describe("<lthn-cred-migration-banner>", () => {
  let cleanup: Array<() => void> = [];
  let originalRefresh: () => Promise<void>;

  beforeEach(() => {
    const proto = LthnCredMigrationBanner.prototype as unknown as RefreshProto;
    originalRefresh = proto._refresh;
    // Neutralise the backend self-fetch so each test drives _status.
    proto._refresh = async () => {};
  });

  afterEach(() => {
    (LthnCredMigrationBanner.prototype as unknown as RefreshProto)._refresh = originalRefresh;
    for (const fn of cleanup) fn();
    cleanup = [];
  });

  it("renders nothing before a status is loaded", async () => {
    const { host, el } = await mountWindow<BannerEl>("lthn-cred-migration-banner");
    cleanup.push(() => host.remove());
    expect(el._status).toBeNull();
    expect(host.querySelector('[role="alert"]')).toBeNull();
  });

  it("renders locked routes from a snake_case status payload", async () => {
    const { host, el } = await mountWindow<BannerEl>("lthn-cred-migration-banner");
    cleanup.push(() => host.remove());

    // Exactly the wire shape Go emits. PascalCase reads would yield
    // undefined → guard slips → .join throws → this test fails.
    el._status = {
      pending_migration_count: 2,
      plaintext_on_disk: true,
      locked_routes: ["openai", "anthropic"],
    };
    await el.updateComplete;

    const banner = host.querySelector('[role="alert"]');
    expect(banner).not.toBeNull();
    expect(banner!.textContent).toContain("openai, anthropic");
  });

  it("renders without throwing when locked_routes is absent (omitempty)", async () => {
    const { host, el } = await mountWindow<BannerEl>("lthn-cred-migration-banner");
    cleanup.push(() => host.remove());

    // count > 0 but locked_routes omitted — the `?? []` guard keeps
    // render() from throwing on a missing optional array.
    el._status = { pending_migration_count: 1, plaintext_on_disk: true };
    await el.updateComplete;

    const banner = host.querySelector('[role="alert"]');
    expect(banner).not.toBeNull();
    expect(banner!.textContent).toContain("(unnamed)");
  });

  it("renders nothing when pending_migration_count is zero", async () => {
    const { host, el } = await mountWindow<BannerEl>("lthn-cred-migration-banner");
    cleanup.push(() => host.remove());

    el._status = { pending_migration_count: 0, plaintext_on_disk: false, locked_routes: [] };
    await el.updateComplete;

    expect(host.querySelector('[role="alert"]')).toBeNull();
  });
});
