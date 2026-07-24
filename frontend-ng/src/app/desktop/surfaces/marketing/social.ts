import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-marketing-social-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MarketingSocialSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'marketing-social',
    title: $localize`:Marketing social title@@surface.marketing.social.title:Social queue`,
    subtitle: $localize`:Marketing social subtitle@@surface.marketing.social.subtitle:4 posts · 2 scheduled`,
    icon: 'share-nodes',
    filters: [
      { id: 'mastodon', label: 'Mastodon' },
      { id: 'x', label: 'X' },
      { id: 'linkedin', label: 'LinkedIn' },
      { id: 'bluesky', label: 'Bluesky' },
    ],
    actions: [{ id: 'compose', label: 'Compose', icon: 'plus', kind: 'add' }],
    rows: [
      {
        id: 'v02-launch',
        title:
          'Lethean v0.2 is out. Tray-first local AI on Apple Silicon at single-watt power draw.',
        meta: 'today · 16:00',
        detail: 'image',
        status: 'scheduled',
        tags: ['mastodon', 'x', 'linkedin'],
      },
      {
        id: 'benchmark-reminder',
        title: 'Every model in lthn ships with a reproducible benchmark, run on your hardware.',
        meta: 'today · 09:00',
        status: 'sent',
        tags: ['mastodon', 'x'],
      },
      {
        id: 'hiring',
        title: "We're hiring a senior Go engineer to work on the lthn runtime.",
        meta: 'tomorrow · 10:00',
        status: 'scheduled',
        tags: ['linkedin'],
      },
      {
        id: 'local-llama-reply',
        title: "We're tracking that one in the backlog as L-203.",
        meta: 'yesterday · 11:14',
        status: 'sent',
        tags: ['mastodon'],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/marketing/social.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'social.update',
    bridgeArgs: [{}],
    liveKeys: ['posts'],
    footer: 'channels: Mastodon, X, LinkedIn, Bluesky · ⌘N to compose · drafts saved locally only',
  };
}
