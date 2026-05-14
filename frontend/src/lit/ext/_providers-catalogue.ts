// SPDX-Licence-Identifier: EUPL-1.2
//
// Shared catalogue of service providers surfaced across:
//   - <lthn-providers-window>     — the marketplace/Extend panel
//   - <lthn-provision-remote-modal> — "don't have a VPS?" recommendations
//
// Affiliate links resolve through https://ref.lthn.ai/{slug} which is
// proxied by link.host.uk.com — every click is tracked so we know which
// recommendations convert. Slugs MUST match what the redirector has
// configured; the frontend never references provider URLs directly.
//
// Adding a provider: append to PROVIDERS below. The category drives
// which group the card appears under in the panel; `affiliate` true
// means "we earn on signup" (shows the small "supports lthn" tag);
// false means "we integrate but no affiliate" (BunnyCDN/ClouDNS are
// upstream-integrated for our hosted services, no kickback).

export type ProviderCategory =
  | "vps"          // virtual servers — where Provision Remote sends users
  | "services"     // first-party Lethean SaaS bundle — host.uk.com/services/*
  | "marketplace"; // first-party apps (CoreAgent, etc.)

export type ProviderStatus =
  | "live"         // first-party product running; or third-party we integrate
  | "external";    // link-only redirect (e.g. VPS affiliate row)

export interface ServiceProvider {
  id:        string;          // stable slug — matches ref.lthn.ai path
  name:      string;
  category:  ProviderCategory;
  status:    ProviderStatus;
  blurb:     string;          // one-line
  icon:      string;          // FA glyph
  affiliate: boolean;         // we earn on signup via the ref link
  firstParty: boolean;        // host.uk.com / lthn.ai-owned service
}

export const PROVIDERS: ServiceProvider[] = [
  // --- VPS — Provision Remote's affiliate row ---
  {
    id: "vps/hetzner", name: "Hetzner Cloud", category: "vps",
    status: "external", icon: "fa-server", affiliate: true, firstParty: false,
    blurb: "Cheap EU VPS — €4/mo for a real machine that can run lthn-agent.",
  },
  {
    id: "vps/digitalocean", name: "DigitalOcean", category: "vps",
    status: "external", icon: "fa-droplet", affiliate: true, firstParty: false,
    blurb: "Easy mode — one-click droplets, predictable pricing, broad regions.",
  },
  {
    id: "vps/vultr", name: "Vultr", category: "vps",
    status: "external", icon: "fa-bolt", affiliate: true, firstParty: false,
    blurb: "Wide region coverage including odd corners — Mumbai, Tel Aviv, Seoul.",
  },

  // --- First-party Lethean SaaS bundle (host.uk.com/services/*) ---
  // Each ships with both an MCP surface (consumed by Lethean Desktop for
  // project data) and a direct REST API (consumed by the user's own apps).
  {
    id: "services/social", name: "Social", category: "services",
    status: "live", icon: "fa-share-nodes", affiliate: false, firstParty: true,
    blurb: "Agency-grade multi-platform social posting + scheduling from one panel — cheap, API-first. social.host.uk.com.",
  },
  {
    id: "services/bio", name: "Bio", category: "services",
    status: "live", icon: "fa-link", affiliate: false, firstParty: true,
    blurb: "Link-in-bio pages, custom domains, attribution-tracked CTAs. link.host.uk.com.",
  },
  {
    id: "services/mail", name: "Mail", category: "services",
    status: "live", icon: "fa-envelope", affiliate: false, firstParty: true,
    blurb: "Transactional + marketing email, SMTP relay, deliverability monitoring. mail.host.uk.com.",
  },
  {
    id: "services/notify", name: "Notify", category: "services",
    status: "live", icon: "fa-bell", affiliate: false, firstParty: true,
    blurb: "Web push, in-app, and broadcast notifications for your sites and apps. notify.host.uk.com.",
  },
  {
    id: "services/trust", name: "Trust", category: "services",
    status: "live", icon: "fa-handshake", affiliate: false, firstParty: true,
    blurb: "Social proof + FOMO campaigns — recent-activity tickers, scarcity, credibility signals. trust.host.uk.com.",
  },
  {
    id: "services/analytics", name: "Analytics", category: "services",
    status: "live", icon: "fa-chart-line", affiliate: false, firstParty: true,
    blurb: "Privacy-respecting web + app analytics. analytics.host.uk.com.",
  },
  {
    id: "services/dns", name: "DNS", category: "services",
    status: "live", icon: "fa-globe", affiliate: false, firstParty: true,
    blurb: "$9.99/yr per zone — DNS hosting, API, control panel, dynamic DNS, free Lethean subdomain. dns.host.uk.com.",
  },
  {
    id: "services/ssl", name: "SSL", category: "services",
    status: "live", icon: "fa-certificate", affiliate: false, firstParty: true,
    blurb: "IP SAN certs from $25/yr, multi-domain SAN bundles, OV/EV options. ssl.host.uk.com.",
  },

  // --- Marketplace apps (first-party) ---
  {
    id: "marketplace/agentic", name: "Agentic", category: "marketplace",
    status: "live", icon: "fa-user-astronaut", affiliate: false, firstParty: true,
    blurb: "Hand-holding automation — scheduled jobs, event-driven workflows, agent orchestration. Open-source. lthn.sh.",
  },
  {
    id: "marketplace/lthn-agent", name: "Lethean Agent", category: "marketplace",
    status: "live", icon: "fa-shield-halved", affiliate: false, firstParty: true,
    blurb: "Bastion-ready agent binary. SSH-installed by Provision Remote.",
  },
];

/** Compose the affiliate-tracked URL for a provider slug. Always
 *  goes through ref.lthn.ai (→ link.host.uk.com) so conversion +
 *  click counts are attributed. */
export function refURL(slug: string): string {
  return `https://ref.lthn.ai/${slug}`;
}

/** Group providers by category for grid rendering. Preserves the
 *  catalogue order within each category. */
export function byCategory(): Record<ProviderCategory, ServiceProvider[]> {
  const out = {} as Record<ProviderCategory, ServiceProvider[]>;
  for (const p of PROVIDERS) {
    (out[p.category] ||= []).push(p);
  }
  return out;
}

/** VPS providers only — what Provision Remote surfaces as
 *  "don't have a server yet?" recommendations. */
export function vpsProviders(): ServiceProvider[] {
  return PROVIDERS.filter(p => p.category === "vps");
}

export const CATEGORY_LABELS: Record<ProviderCategory, string> = {
  vps:         "Virtual servers",
  services:    "Lethean services",
  marketplace: "Lethean apps",
};

export const CATEGORY_BLURBS: Record<ProviderCategory, string> = {
  vps:         "Where Provision Remote sends your fleet expansion.",
  services:    "First-party Lethean SaaS — API-first, MCP-integrated, composable into your own projects.",
  marketplace: "First-party Lethean apps you can install on the fleet.",
};
