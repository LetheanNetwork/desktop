import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-sales-contacts-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class SalesContactsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'sales-contacts',
    title: $localize`:Sales contacts title@@surface.sales.contacts.title:Contacts`,
    subtitle: $localize`:Sales contacts subtitle@@surface.sales.contacts.subtitle:6 active · 3 hot`,
    icon: 'address-book',
    actions: [{ id: 'new', label: 'Add contact', icon: 'plus', kind: 'add' }],
    filters: [
      { id: 'hot', label: 'Hot' },
      { id: 'warm', label: 'Warm' },
      { id: 'cool', label: 'Cool' },
    ],
    rows: [
      {
        id: 'ada-penley',
        title: 'Ada Penley',
        meta: 'CTO · Heritage Law',
        detail: 'Next · call Friday',
        status: 'hot',
        value: 'replied 2 d',
      },
      {
        id: 'marcus-stannard',
        title: 'Marcus Stannard',
        meta: 'Partner · Stannard & Co',
        detail: 'Next · pilot sign-off',
        status: 'warm',
        value: 'emailed 5 d',
      },
      {
        id: 'imogen-beck',
        title: 'Dr Imogen Beck',
        meta: 'CIO · Lichfield NHS Trust',
        detail: 'Next · DPIA review',
        status: 'warm',
        value: 'meeting 8 d',
      },
      {
        id: 'tom-pemberton',
        title: 'Tom Pemberton',
        meta: 'COO · Pemberton Capital',
        detail: 'Next · re-engage Q3',
        status: 'cool',
        value: 'replied 3 w',
      },
      {
        id: 'sarah-whitethorn',
        title: 'Sarah Whitethorn',
        meta: 'Founder · Whitethorn Press',
        detail: 'Next · contract',
        status: 'hot',
        value: 'replied 1 d',
      },
      {
        id: 'david-crown',
        title: 'David Crown',
        meta: 'IT Director · Crown Estates',
        detail: 'Next · SOW review',
        status: 'hot',
        value: 'replied 4 d',
      },
    ],
    searchPlaceholder: 'Search name or organisation',
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/sales/contacts.Service.List',
    conflictReloadService: 'sales.contacts.update',
    bridgeArgs: [{}],
    liveKeys: ['contacts'],
    footer: 'warmth recomputed weekly from last touch · ⌘N to add · CRM stays local',
  };
}
