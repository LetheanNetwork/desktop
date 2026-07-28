import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-marketing-content-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MarketingContentSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'marketing-content',
    title: $localize`:Marketing content title@@surface.marketing.content.title:Content calendar`,
    subtitle: $localize`:Marketing content subtitle@@surface.marketing.content.subtitle:6 posts in flight · 1 due today`,
    icon: 'calendar-days',
    kind: 'board',
    actions: [{ id: 'new', label: 'Write', icon: 'plus', kind: 'add' }],
    searchPlaceholder: 'Filter content cards',
    columns: [
      {
        id: 'idea',
        title: 'Ideas',
        cards: [
          { id: 'watt-token', title: 'Watt-per-token comparison post', meta: 'you' },
          { id: 'claude-code', title: 'Tutorial: connect lthn to Claude Code', meta: 'Mei' },
        ],
      },
      {
        id: 'draft',
        title: 'Drafting',
        cards: [
          {
            id: 'release-notes',
            title: 'v0.2 release notes',
            meta: 'you',
            value: 'due today',
          },
          { id: 'local-ai', title: 'State of local AI · long-read', meta: 'Tobi' },
        ],
      },
      {
        id: 'review',
        title: 'Review',
        cards: [{ id: 'eupl', title: 'Why we chose EUPL-1.2', meta: 'you → Mei' }],
      },
      {
        id: 'ready',
        title: 'Ready',
        cards: [{ id: 'manifesto', title: 'Sovereign-compute manifesto', meta: 'you' }],
      },
      {
        id: 'live',
        title: 'Live',
        cards: [
          { id: 'tray-retro', title: 'Tray-app architecture · v0.1 retro', meta: '3 d ago' },
          { id: 'hiring', title: 'Hiring · senior Go engineer', meta: '1 w ago' },
        ],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/marketing/content.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'content.update',
    bridgeArgs: [{}],
    liveKeys: ['items'],
    footer: 'drag cards to advance · ⌘N to write · publishes to lthn.ai/blog',
  };
}
