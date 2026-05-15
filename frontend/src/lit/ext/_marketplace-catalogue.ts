// SPDX-Licence-Identifier: EUPL-1.2
//
// Local cache of the marketplace registry index. Mirrors the shape of
// _providers-catalogue.ts but for lthn-vm bundles. The catalogue is
// seeded from FetchIndex() at window open and stored in module state
// for the lifetime of the window — no cross-window sharing needed.
//
// v1: FetchIndex returns server-side data from marketplace.lthn.ai/v1/
// (24 h TTL, cached on disk by pkg/marketplace/registry.go). This
// module holds the in-memory snapshot for the duration of a session.

import { FetchIndex } from "@desktop/marketplace/service";

export interface CatalogueEntry {
  name:            string;
  display:         string;
  description:     string;
  category:        string;
  icon:            string;
  homepage:        string;
  license:         string;
  sourceURL:       string;
  requiresVersion: string;
  lastUpdated:     string;
}

export interface CatalogueIndex {
  entries:    CatalogueEntry[];
  cachedAt:   string;
  fromCache:  boolean;
}

// CATEGORY_LABELS maps category slugs to display strings.
export const CATEGORY_LABELS: Record<string, string> = {
  "ai-agents":  "AI Agents",
  "databases":  "Databases",
  "devtools":   "Dev Tools",
  "security":   "Security",
  "monitoring": "Monitoring",
  "automation": "Automation",
  "storage":    "Storage",
};

export function categoryLabel(cat: string): string {
  return CATEGORY_LABELS[cat] ?? cat;
}

// fetchCatalogue downloads the live index via Wails. Returns an empty
// index on failure so callers never need to null-check.
export async function fetchCatalogue(): Promise<CatalogueIndex> {
  try {
    const r = await FetchIndex("");
    if (!r.OK) return { entries: [], cachedAt: "", fromCache: false };
    const raw = r.Value as {
      Entries?: CatalogueEntry[];
      CachedAt?: string;
      FromCache?: boolean;
    };
    return {
      entries:   raw.Entries   ?? [],
      cachedAt:  raw.CachedAt  ?? "",
      fromCache: raw.FromCache ?? false,
    };
  } catch {
    return { entries: [], cachedAt: "", fromCache: false };
  }
}

// filterCatalogue narrows the entry list by query text + category slug.
// Query is case-insensitive; matches name, display, or description.
export function filterCatalogue(
  catalogue: CatalogueEntry[],
  query: string,
  category: string,
): CatalogueEntry[] {
  const q = query.toLowerCase();
  return catalogue.filter(e => {
    if (category && e.category !== category) return false;
    if (!q) return true;
    return (
      e.name.toLowerCase().includes(q) ||
      e.display.toLowerCase().includes(q) ||
      e.description.toLowerCase().includes(q)
    );
  });
}

// categories returns the sorted unique category slugs present in the catalogue.
export function categories(catalogue: CatalogueEntry[]): string[] {
  const s = new Set<string>();
  for (const e of catalogue) if (e.category) s.add(e.category);
  return Array.from(s).sort((a, b) => a.localeCompare(b));
}
