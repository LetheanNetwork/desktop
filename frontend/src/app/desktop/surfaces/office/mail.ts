import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-office-mail-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OfficeMailSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'office-mail',
    title: $localize`:Office mail title@@surface.office.mail.title:Mail`,
    subtitle: $localize`:Office mail subtitle@@surface.office.mail.subtitle:24 unread`,
    icon: 'envelope',
    metrics: [
      { label: 'Inbox', value: '24', tone: 'brand' },
      { label: 'Drafts', value: '3' },
      { label: 'Threads', value: '5' },
    ],
    filters: [
      { id: 'unread', label: 'Unread' },
      { id: 'read', label: 'Read' },
    ],
    rows: [
      {
        id: 'ada',
        title: 'Re: SOW v2 — DPO feedback',
        meta: 'Ada Penley · now',
        detail: "Looking forward to seeing the proposal. We'll need to loop in our DPO…",
        status: 'unread',
      },
      {
        id: 'github',
        title: '[lethean/desktop] #482 reviewed',
        meta: 'GitHub · 14:32',
        detail: 'Tobi requested changes on your pull request. 3 comments · 2 files.',
        status: 'unread',
      },
      {
        id: 'calliope',
        title: 'Pitch follow-up',
        meta: 'Calliope VC · yesterday',
        detail: 'Thanks for the call. Sending the diligence questionnaire later this week…',
        status: 'read',
      },
      {
        id: 'mei',
        title: 'Linux packaging — blocker',
        meta: 'Mei (team) · yesterday',
        detail: 'Hit a snag with the AppImage signing flow. Have a workaround…',
        status: 'read',
      },
      {
        id: 'local-llama',
        title: 'Your post got 412 upvotes',
        meta: 'r/LocalLLaMA · 2 d',
        detail: 'You reached the front page. Top comment thread now has 84 replies…',
        status: 'read',
      },
    ],
    sections: [
      {
        title: 'Vi triage',
        body: 'The DPO feedback thread looks time-sensitive. Offer a concise SOW reply?',
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/office/mail.Service.ListThreads',
    pollMs: 60_000,
    bridgeArgs: [{}],
    liveKeys: ['threads'],
    footer: '5 threads · IMAP via host.uk.com · Vi triage on-device only',
  };
}
