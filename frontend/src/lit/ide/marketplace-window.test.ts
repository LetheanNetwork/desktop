// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow } from "../../test/window-fixture";
import "./marketplace-window";

interface MarketplaceTestElement extends LitElement {
  catalogue: Array<{ code: string; name: string; category?: string }>;
  _categories(): string[];
}

describe("lthn-marketplace-window", () => {
  it("sorts unique categories with locale ordering", async () => {
    const { el } = await mountWindow<MarketplaceTestElement>("lthn-marketplace-window");
    el.catalogue = [
      { code: "z", name: "Z", category: "zeta" },
      { code: "a", name: "A", category: "alpha" },
      { code: "u", name: "U" },
      { code: "ae", name: "AE", category: "äether" },
      { code: "again", name: "Again", category: "alpha" },
    ];

    expect(el._categories()).toEqual(["äether", "alpha", "zeta"]);
  });
});
