// apps/lethernet.app.ts — dumb app-view. Peer fleet table.
import { Component, CUSTOM_ELEMENTS_SCHEMA, Input, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win } from '../desktop.data';

@Component({
  selector: 'lthn-lethernet-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="appbody">
      <div class="ctoolbar">
        <h1 i18n="LetherNet view heading@@lethernet.heading">LetherNet fleet</h1>
        <lthn-badge variant="ok" i18n="LetherNet peer count@@lethernet.peerCount"
          >4 peers</lthn-badge
        >
      </div>
      <div class="tiles c3">
        <lthn-card pad="11"
          ><lthn-stat
            value="4"
            label="Peers online"
            i18n-label="Online peer metric@@lethernet.peersOnline"
            mono
          ></lthn-stat
        ></lthn-card>
        <lthn-card pad="11"
          ><lthn-stat
            value="286"
            label="tok/s pooled"
            i18n-label="Pooled throughput metric@@lethernet.pooledThroughput"
            mono
          ></lthn-stat
        ></lthn-card>
        <lthn-card pad="11"
          ><lthn-stat
            value="2"
            label="Regions"
            i18n-label="Region count metric@@lethernet.regions"
            mono
          ></lthn-stat
        ></lthn-card>
      </div>
      <lthn-datatable [attr.columns]="cols" [attr.rows]="rows"></lthn-datatable>
      <p
        style="font-size:12px;color:var(--fg-3);margin:12px 0 0"
        i18n="LetherNet routing explanation@@lethernet.routingDescription"
      >
        Requests route to the least-loaded peer serving the requested model. Weights never move —
        only prompts and completions.
      </p>
    </div>
  `,
})
export class LetherNetApp implements AppView {
  @Input() win!: Win;
  cols = JSON.stringify([
    {
      key: 'peer',
      label: $localize`:LetherNet table column@@lethernet.column.peer:Peer`,
      type: 'mono',
    },
    {
      key: 'region',
      label: $localize`:LetherNet table column@@lethernet.column.region:Region`,
      type: 'mono',
    },
    { key: 'model', label: $localize`:LetherNet table column@@lethernet.column.serving:Serving` },
    {
      key: 'state',
      label: $localize`:LetherNet table column@@lethernet.column.state:State`,
      type: 'status',
    },
  ]);
  rows =
    '[{"peer":"vi-01.lan","region":"eu-west-2","model":"llama-3.1-70b","state":"running"},{"peer":"vi-02.lan","region":"eu-west-2","model":"mistral-small","state":"active"},{"peer":"hoplite-7","region":"eu-west-1","model":"—","state":"idle"},{"peer":"studio.local","region":"local","model":"gemma-2-27b","state":"running"}]';
}
