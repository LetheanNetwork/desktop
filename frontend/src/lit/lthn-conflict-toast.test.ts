// SPDX-Licence-Identifier: EUPL-1.2

import { afterEach, describe, expect, it } from "vitest";
import { mountWindow } from "../test/window-fixture";
import {
  CONFLICT_409_EVENT,
  CONFLICT_RELOAD_EVENT,
  type ConflictDetail,
} from "./conflict-dispatch";
import { entityLabelFor } from "./lthn-conflict-toast";

// Side-effect import — registers <lthn-conflict-toast>.
import "./lthn-conflict-toast";

interface ToastEl extends HTMLElement {
  _active: ConflictDetail | null;
  updateComplete: Promise<boolean>;
}

describe("entityLabelFor (pure helper)", () => {
  it("maps known sales services to friendly labels", () => {
    expect(entityLabelFor("sales.contacts.update")).toBe("Contacts");
    expect(entityLabelFor("sales.deals.update")).toBe("Deal");
    expect(entityLabelFor("sales.pipeline.update")).toBe("Pipeline");
  });
  it("title-cases unknown entity segments", () => {
    expect(entityLabelFor("sales.forecast.update")).toBe("Forecast");
  });
  it("falls back to 'Record' for malformed identifiers", () => {
    expect(entityLabelFor("")).toBe("Record");
    expect(entityLabelFor("single")).toBe("Record");
    expect(entityLabelFor(undefined)).toBe("Record");
  });
});

describe("<lthn-conflict-toast>", () => {
  let cleanup: Array<() => void> = [];

  afterEach(() => {
    for (const fn of cleanup) fn();
    cleanup = [];
  });

  it("renders nothing by default (no conflict yet)", async () => {
    const { host, el } = await mountWindow<ToastEl>("lthn-conflict-toast");
    cleanup.push(() => host.remove());
    expect(el._active).toBeNull();
    expect(host.querySelector(".lthn-conflict-toast")).toBeNull();
  });

  it("renders on CONFLICT_409_EVENT with the entity label from service detail", async () => {
    const { host, el } = await mountWindow<ToastEl>("lthn-conflict-toast");
    cleanup.push(() => host.remove());

    window.dispatchEvent(new CustomEvent<ConflictDetail>(CONFLICT_409_EVENT, {
      detail: {
        service: "sales.contacts.update",
        errorCode: "sales.contacts.update.conflict",
        currentVersion: 3,
      },
    }));
    await el.updateComplete;

    const toast = host.querySelector(".lthn-conflict-toast");
    expect(toast).not.toBeNull();
    expect(toast!.textContent).toContain("Contacts");
    expect(toast!.textContent).toContain("Reload to see the current version");
  });

  it("Reload button fires CONFLICT_RELOAD_EVENT carrying the original detail", async () => {
    const { host, el } = await mountWindow<ToastEl>("lthn-conflict-toast");
    cleanup.push(() => host.remove());

    let observed: ConflictDetail | null = null;
    const listener = (e: Event) => {
      observed = (e as CustomEvent<ConflictDetail>).detail;
    };
    window.addEventListener(CONFLICT_RELOAD_EVENT, listener);
    cleanup.push(() => window.removeEventListener(CONFLICT_RELOAD_EVENT, listener));

    window.dispatchEvent(new CustomEvent<ConflictDetail>(CONFLICT_409_EVENT, {
      detail: { service: "sales.deals.update", currentVersion: 4 },
    }));
    await el.updateComplete;

    const reload = host.querySelector(".lthn-conflict-toast-reload") as HTMLButtonElement;
    expect(reload).not.toBeNull();
    reload.click();
    await el.updateComplete;

    expect(observed).not.toBeNull();
    expect(observed!.service).toBe("sales.deals.update");
    expect(observed!.currentVersion).toBe(4);
    // Toast should be dismissed after Reload.
    expect(host.querySelector(".lthn-conflict-toast")).toBeNull();
  });

  it("Dismiss button hides toast without firing CONFLICT_RELOAD_EVENT", async () => {
    const { host, el } = await mountWindow<ToastEl>("lthn-conflict-toast");
    cleanup.push(() => host.remove());

    let observedReload = false;
    const listener = () => { observedReload = true; };
    window.addEventListener(CONFLICT_RELOAD_EVENT, listener);
    cleanup.push(() => window.removeEventListener(CONFLICT_RELOAD_EVENT, listener));

    window.dispatchEvent(new CustomEvent<ConflictDetail>(CONFLICT_409_EVENT, {
      detail: { service: "sales.pipeline.update" },
    }));
    await el.updateComplete;

    const dismiss = host.querySelector(".lthn-conflict-toast-dismiss") as HTMLButtonElement;
    expect(dismiss).not.toBeNull();
    dismiss.click();
    await el.updateComplete;

    expect(observedReload).toBe(false);
    expect(host.querySelector(".lthn-conflict-toast")).toBeNull();
  });
});
