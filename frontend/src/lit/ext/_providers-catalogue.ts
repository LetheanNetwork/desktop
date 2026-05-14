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
  | "cdn"          // content delivery
  | "dns"          // domain + DNS
  | "ssl"          // certificates
  | "marketplace"  // first-party apps (CoreAgent, etc.)
  | "comms"        // social / push / messaging surfaces
  | "analytics";   // tracking / observability

export type ProviderStatus =
  | "live"         // integrated + working
  | "coming-soon"  // surface exists, integration in flight
  | "external";    // link-only, no integration (yet)

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

  // --- CDN / DNS / SSL — third-party integrations ---
  {
    id: "cdn/bunny", name: "BunnyCDN", category: "cdn",
    status: "live", icon: "fa-rabbit", affiliate: false, firstParty: false,
    blurb: "Edge CDN we integrate against for hosted-app delivery.",
  },
  {
    id: "dns/cloudns", name: "ClouDNS", category: "dns",
    status: "live", icon: "fa-cloud", affiliate: false, firstParty: false,
    blurb: "Programmable DNS — what we drive when you point a domain at a Lethean app.",
  },
  {
    id: "ssl/gogetssl", name: "GoGetSSL", category: "ssl",
    status: "live", icon: "fa-lock", affiliate: false, firstParty: false,
    blurb: "Cheap commercial SSL when Let's Encrypt isn't enough.",
  },

  // --- First-party HostUK / Lethean services ---
  {
    id: "comms/social", name: "Social", category: "comms",
    status: "coming-soon", icon: "fa-share-nodes", affiliate: false, firstParty: true,
    blurb: "Social-media posting, scheduling, multi-account from one panel. social.host.uk.com.",
  },
  {
    id: "analytics/track", name: "Analytics", category: "analytics",
    status: "coming-soon", icon: "fa-chart-line", affiliate: false, firstParty: true,
    blurb: "Privacy-respecting web + app analytics. analytics.host.uk.com.",
  },
  {
    id: "comms/push", name: "Push & FOMO", category: "comms",
    status: "coming-soon", icon: "fa-bell", affiliate: false, firstParty: true,
    blurb: "Web push notifications + FOMO campaign primitives for your apps.",
  },

  // --- Marketplace apps (first-party) ---
  {
    id: "marketplace/coreagent", name: "CoreAgent", category: "marketplace",
    status: "coming-soon", icon: "fa-user-astronaut", affiliate: false, firstParty: true,
    blurb: "Headless agent runtime — runs on any paired machine, takes work from the fleet.",
  },
  {
    id: "marketplace/lthn-agent", name: "Lethean Agent", category: "marketplace",
    status: "coming-soon", icon: "fa-shield-halved", affiliate: false, firstParty: true,
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
  cdn:         "Content delivery",
  dns:         "DNS & domains",
  ssl:         "Certificates",
  marketplace: "Lethean apps",
  comms:       "Communications",
  analytics:   "Analytics",
};

export const CATEGORY_BLURBS: Record<ProviderCategory, string> = {
  vps:         "Where Provision Remote sends your fleet expansion.",
  cdn:         "Edge delivery we drive for hosted Lethean apps.",
  dns:         "Programmable DNS for domain routing.",
  ssl:         "Cheap commercial certs when Let's Encrypt isn't the right fit.",
  marketplace: "First-party Lethean apps you can install on the fleet.",
  comms:       "Social posting, push notifications, campaign primitives.",
  analytics:   "Privacy-respecting tracking for your apps and sites.",
};
