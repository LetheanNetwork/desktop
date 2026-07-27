// SPDX-License-Identifier: EUPL-1.2

import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import type { DesktopDataResource } from '../../desktop-data-resource';
import { DesktopDataStatusView } from '../../desktop-data-status.view';
import type {
  ControlServiceIntent,
  DesktopRestartPolicy,
  DesktopServiceCatalogue,
  DesktopServiceOutput,
  DesktopServiceSnapshot,
  DesktopServiceState,
} from './control-services.models';

@Component({
  selector: 'lthn-control-services-view',
  standalone: true,
  imports: [DesktopDataStatusView],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="services" aria-labelledby="control-services-heading">
      <header class="services__header">
        <div>
          <h2 id="control-services-heading" i18n="Services heading@@control.services.heading">
            Services
          </h2>
          <p i18n="Services lifecycle explanation@@control.services.explanation">
            Starts manually · stops when Lethean Desktop quits.
          </p>
        </div>
        <lthn-desktop-data-status [status]="resource()" (retry)="retry.emit()" />
      </header>

      @if (services().length) {
        <div class="services__list" aria-live="polite">
          @for (service of services(); track service.definition.id) {
            <article
              class="service"
              [attr.data-service-id]="service.definition.id"
              [attr.aria-busy]="isPending(service.definition.id)"
            >
              <span
                class="service__indicator"
                [attr.data-state]="service.state"
                aria-hidden="true"
              ></span>
              <div class="service__identity">
                <div class="service__title">
                  <h3>{{ service.definition.displayName }}</h3>
                  <code>{{ service.definition.id }}</code>
                </div>
                <p>{{ service.definition.description }}</p>
                <dl>
                  <div>
                    <dt i18n="Service kind label@@control.services.kind">Kind</dt>
                    <dd>{{ service.definition.kind }}</dd>
                  </div>
                  <div>
                    <dt i18n="Service owner label@@control.services.owner">Owner</dt>
                    <dd>{{ service.definition.owner }}</dd>
                  </div>
                  <div>
                    <dt i18n="Service restart policy label@@control.services.restartPolicy">
                      Restart
                    </dt>
                    <dd>{{ policyLabel(service.definition.restartPolicy) }}</dd>
                  </div>
                  @if (service.pid > 0) {
                    <div>
                      <dt i18n="Service PID label@@control.services.pid">PID</dt>
                      <dd>{{ service.pid }}</dd>
                    </div>
                  }
                </dl>
                @if (service.lastError; as failure) {
                  <p class="service__failure" role="status">
                    {{ failure.message }}
                  </p>
                }
              </div>
              <div class="service__controls">
                <span class="service__state" [attr.data-state]="service.state">
                  {{ stateLabel(service.state) }}
                </span>
                @if (isPending(service.definition.id)) {
                  <span class="service__pending" role="status">Working…</span>
                }
                <div class="service__actions">
                  @if (canStart(service)) {
                    <button
                      type="button"
                      data-action="start"
                      [disabled]="isPending(service.definition.id)"
                      (click)="emitAction('start', service.definition.id)"
                    >
                      <span i18n="Start service action@@control.services.start">Start</span>
                    </button>
                  }
                  @if (canStop(service)) {
                    <button
                      type="button"
                      data-action="stop"
                      [disabled]="isPending(service.definition.id)"
                      (click)="emitAction('stop', service.definition.id)"
                    >
                      <span i18n="Stop service action@@control.services.stop">Stop</span>
                    </button>
                  }
                  @if (canRestart(service)) {
                    <button
                      type="button"
                      data-action="restart"
                      [disabled]="isPending(service.definition.id)"
                      (click)="emitAction('restart', service.definition.id)"
                    >
                      <span i18n="Restart service action@@control.services.restart">Restart</span>
                    </button>
                  }
                  @if (service.processId) {
                    <button
                      type="button"
                      data-action="output"
                      [disabled]="isPending(service.definition.id)"
                      (click)="emitAction('output', service.definition.id)"
                    >
                      <span i18n="Read service output action@@control.services.output">Output</span>
                    </button>
                  }
                </div>
              </div>
            </article>
          }
        </div>
      } @else {
        <div class="services__empty">
          <h3 i18n="No services heading@@control.services.emptyHeading">
            No services are available
          </h3>
          <p i18n="No services explanation@@control.services.emptyExplanation">
            Optional capabilities appear here when their trusted definitions are registered.
          </p>
        </div>
      }

      @if (outputView(); as output) {
        <section class="services__output" aria-live="polite">
          <div>
            <h3>{{ output.id }} output</h3>
            <span>
              generation {{ output.generation }}
              @if (output.truncated) {
                · tail truncated
              }
            </span>
          </div>
          <pre>{{ output.output }}</pre>
        </section>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }
    .services {
      display: grid;
      gap: 12px;
    }
    .services__header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
    }
    .services__header h2,
    .services__empty h3,
    .services__output h3,
    .service h3 {
      margin: 0;
      color: var(--fg-0);
    }
    .services__header h2 {
      font-size: 15px;
      font-weight: 600;
    }
    .services__header p,
    .services__empty p {
      margin: 4px 0 0;
      color: var(--fg-3);
      font-size: 11.5px;
    }
    .services__list {
      display: grid;
      gap: 8px;
    }
    .service {
      display: grid;
      grid-template-columns: 8px minmax(0, 1fr) auto;
      gap: 12px;
      padding: 12px 13px;
      border: 1px solid var(--line-1);
      border-radius: 10px;
      background: var(--ink-2);
    }
    .service__indicator {
      align-self: stretch;
      width: 3px;
      min-height: 36px;
      border-radius: 999px;
      background: var(--fg-4);
    }
    .service__indicator[data-state='running'] {
      background: var(--ok-400, #54c7a0);
      box-shadow: 0 0 10px color-mix(in oklch, var(--ok-400, #54c7a0) 45%, transparent);
    }
    .service__indicator[data-state='starting'],
    .service__indicator[data-state='stopping'] {
      background: var(--warning-400, #febc2e);
    }
    .service__indicator[data-state='failed'] {
      background: var(--danger-400);
    }
    .service__identity {
      min-width: 0;
    }
    .service__title {
      display: flex;
      align-items: baseline;
      gap: 9px;
    }
    .service__title h3 {
      overflow: hidden;
      font-size: 13px;
      font-weight: 600;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .service__title code,
    .service dl,
    .service__pending,
    .services__output span {
      color: var(--fg-3);
      font-family: var(--font-mono);
      font-size: 10px;
    }
    .service__identity > p {
      margin: 4px 0 8px;
      color: var(--fg-3);
      font-size: 11.5px;
      line-height: 1.4;
    }
    .service dl {
      display: flex;
      flex-wrap: wrap;
      gap: 5px 13px;
      margin: 0;
    }
    .service dl div {
      display: flex;
      gap: 5px;
    }
    .service dt {
      color: var(--fg-4);
    }
    .service dd {
      margin: 0;
      color: var(--fg-2);
    }
    .service__identity .service__failure {
      margin-bottom: 0;
      color: var(--danger-400);
    }
    .service__controls {
      display: flex;
      min-width: 176px;
      flex-direction: column;
      align-items: flex-end;
      gap: 7px;
    }
    .service__state {
      border: 1px solid var(--line-2);
      border-radius: 999px;
      padding: 2px 8px;
      color: var(--fg-2);
      font-family: var(--font-mono);
      font-size: 10px;
    }
    .service__state[data-state='running'] {
      border-color: color-mix(in oklch, var(--ok-400, #54c7a0) 35%, transparent);
      color: var(--ok-400, #54c7a0);
    }
    .service__state[data-state='failed'] {
      border-color: color-mix(in oklch, var(--danger-400) 35%, transparent);
      color: var(--danger-400);
    }
    .service__actions {
      display: flex;
      flex-wrap: wrap;
      justify-content: flex-end;
      gap: 5px;
    }
    .service__actions button {
      border: 1px solid var(--line-2);
      border-radius: 6px;
      background: var(--ink-3);
      color: var(--fg-1);
      cursor: pointer;
      font: inherit;
      font-size: 11px;
      padding: 4px 8px;
    }
    .service__actions button:hover:not(:disabled) {
      border-color: color-mix(in oklch, var(--brand-400) 55%, var(--line-2));
      color: var(--fg-0);
    }
    .service__actions button:disabled {
      cursor: progress;
      opacity: 0.5;
    }
    .services__empty,
    .services__output {
      border: 1px solid var(--line-1);
      border-radius: 10px;
      background: var(--ink-2);
      padding: 16px;
    }
    .services__empty h3,
    .services__output h3 {
      font-size: 12.5px;
    }
    .services__output > div {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 8px;
    }
    .services__output pre {
      max-height: 180px;
      margin: 0;
      overflow: auto;
      border: 1px solid var(--line-1);
      border-radius: 7px;
      background: var(--ink-0);
      color: var(--fg-2);
      font: 11px/1.55 var(--font-mono);
      padding: 10px;
      white-space: pre-wrap;
      word-break: break-word;
    }
    @media (max-width: 720px) {
      .services__header {
        flex-direction: column;
      }
      .service {
        grid-template-columns: 6px minmax(0, 1fr);
      }
      .service__controls {
        grid-column: 2;
        min-width: 0;
        align-items: flex-start;
      }
      .service__actions {
        justify-content: flex-start;
      }
    }
  `,
})
export class ControlServicesView {
  readonly resource = input.required<DesktopDataResource<DesktopServiceCatalogue>>();
  readonly pendingIds = input<readonly string[]>([]);
  readonly outputView = input<DesktopServiceOutput | null>(null);
  readonly action = output<ControlServiceIntent>();
  readonly retry = output<void>();

  readonly services = computed(() => this.resource().value?.services ?? []);
  private readonly pending = computed(() => new Set(this.pendingIds()));

  isPending(id: string): boolean {
    return this.pending().has(id);
  }

  canStart(service: DesktopServiceSnapshot): boolean {
    return (
      !service.desired &&
      (service.state === 'stopped' || service.state === 'exited' || service.state === 'failed')
    );
  }

  canStop(service: DesktopServiceSnapshot): boolean {
    return service.desired || service.state === 'starting' || service.state === 'running';
  }

  canRestart(service: DesktopServiceSnapshot): boolean {
    return service.state === 'running';
  }

  emitAction(kind: ControlServiceIntent['kind'], id: string): void {
    this.action.emit({ kind, id });
  }

  stateLabel(state: DesktopServiceState): string {
    switch (state) {
      case 'stopped':
        return $localize`:Stopped service state@@control.services.state.stopped:Stopped`;
      case 'starting':
        return $localize`:Starting service state@@control.services.state.starting:Starting`;
      case 'running':
        return $localize`:Running service state@@control.services.state.running:Running`;
      case 'stopping':
        return $localize`:Stopping service state@@control.services.state.stopping:Stopping`;
      case 'exited':
        return $localize`:Exited service state@@control.services.state.exited:Exited`;
      case 'failed':
        return $localize`:Failed service state@@control.services.state.failed:Failed`;
    }
  }

  policyLabel(policy: DesktopRestartPolicy): string {
    switch (policy) {
      case 'never':
        return $localize`:Manual service restart policy@@control.services.policy.never:manual`;
      case 'on-failure':
        return $localize`:On failure service restart policy@@control.services.policy.onFailure:on failure`;
      case 'always':
        return $localize`:Always service restart policy@@control.services.policy.always:always`;
    }
  }
}
