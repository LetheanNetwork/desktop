// apps/activity.app.ts — example dumb app-view (streaming logs). Self-contained;
// swap `lines` for an @Input fed by a live socket in production.
import { Component, CUSTOM_ELEMENTS_SCHEMA, Input, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win } from '../desktop.data';

@Component({
  selector: 'lthn-activity-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="logwrap">
      <div class="logs">
        <div
          class="ln"
          *ngFor="let l of lines"
          [class.warn]="l.sev === 'warn'"
          [class.err]="l.sev === 'err'"
        >
          <span class="t">{{ l.t }}</span
          ><span class="c">{{ l.c }}</span
          ><span class="m">{{ l.m }}</span>
        </div>
      </div>
      <div class="logfoot" i18n="Activity live-tail privacy status@@activity.liveTailStatus">
        <lthn-status-dot variant="ok" pulse></lthn-status-dot> Live tail · all local — never leaves
        this machine
      </div>
    </div>
  `,
})
export class ActivityApp implements AppView {
  @Input() win!: Win;
  @Input() lines: { t: string; c: string; m: string; sev: string }[] = [
    { t: '09:14:02', c: 'runner', m: 'model llama-3.1-70b loaded (18.4 GB)', sev: '' },
    { t: '09:14:03', c: 'api', m: 'listening on 127.0.0.1:1988', sev: '' },
    { t: '09:15:11', c: 'gen', m: 'req#8841 · 512 tok · 34.2 tok/s', sev: '' },
    { t: '09:15:44', c: 'cache', m: 'KV-cache 62% — trimming oldest turn', sev: 'warn' },
    { t: '09:16:02', c: 'gen', m: 'req#8842 · 1,024 tok · 33.8 tok/s', sev: '' },
    { t: '09:16:39', c: 'net', m: 'peer vi-02.lan joined LetherNet', sev: '' },
    { t: '09:17:20', c: 'runner', m: 'mistral-small warm-start failed, retrying', sev: 'err' },
    { t: '09:17:22', c: 'runner', m: 'mistral-small active', sev: '' },
    { t: '09:18:01', c: 'gen', m: 'req#8843 · 256 tok · 88.1 tok/s', sev: '' },
  ];
}
