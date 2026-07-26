// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Directive,
  ElementRef,
  Input,
  Renderer2,
  input,
  output,
} from '@angular/core';
import { DesktopDataStateBadge } from '../../desktop-data-state-badge';
import type { DesktopDataState } from '../../desktop-data-state';
import type { ControlSettingsViewModel } from './control-view.models';

@Directive({
  selector: 'lthn-toggle',
  standalone: true,
})
export class LthnToggleOnDirective {
  constructor(
    private readonly element: ElementRef<HTMLElement>,
    private readonly renderer: Renderer2,
  ) {}

  @Input() set on(value: boolean) {
    if (value) {
      this.renderer.setAttribute(this.element.nativeElement, 'on', '');
      return;
    }
    this.renderer.removeAttribute(this.element.nativeElement, 'on');
  }
}

@Component({
  selector: 'lthn-control-settings-view',
  standalone: true,
  imports: [DesktopDataStateBadge, LthnToggleOnDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="Configuration view heading@@control.config.heading">Configuration</h1>
      <lthn-desktop-data-state [state]="dataState()" />
      <span class="cfgsrc" i18n="Configuration precedence@@control.config.precedence">
        Defaults → File → Env → Set
      </span>
      <button class="nbtn" (click)="commit.emit()">
        <lthn-icon name="check" size="10"></lthn-icon>
        <span i18n="Commit configuration action@@control.config.commit">Commit</span>
      </button>
    </div>
    @for (group of model().groups; track group.name) {
      <div class="cfggrp">
        <span class="glab">{{ group.name }}</span>
        @for (row of group.rows; track row.key) {
          <div class="cfgrow">
            <div class="cfgk">
              <b>{{ row.key }}</b>
              <span class="src {{ row.source }}">{{ sourceLabel(row.source) }}</span>
            </div>
            <input class="cfgin" [value]="row.value" [attr.aria-label]="row.key" />
          </div>
        }
      </div>
    }
    <div class="cfggrp">
      <span class="glab" i18n="Feature flag settings group@@control.config.featureFlags">
        Feature flags
      </span>
      @for (flag of model().flags; track flag.key) {
        <div class="cfgrow">
          <div class="cfgk">
            <b>{{ flag.key }}</b>
            <span class="src {{ flag.source }}">{{ sourceLabel(flag.source) }}</span>
          </div>
          <lthn-toggle [on]="flag.on"></lthn-toggle>
        </div>
      }
    </div>
    <p class="cfghint" i18n="Configuration environment override help@@control.config.envHelp">
      <lthn-icon name="circle-info" size="11"></lthn-icon>
      Env overrides use <code>CORE_CONFIG_*</code>; environment values are never written back on
      Commit (dual-Viper).
    </p>
  `,
})
export class ControlSettingsView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlSettingsViewModel>();
  readonly commit = output<void>();

  sourceLabel(source: string): string {
    const labels: Record<string, string> = {
      default: $localize`:Configuration source@@control.config.source.default:default`,
      file: $localize`:Configuration source@@control.config.source.file:file`,
      env: $localize`:Configuration source@@control.config.source.env:env`,
      set: $localize`:Configuration source@@control.config.source.set:set`,
    };
    return labels[source] ?? source;
  }
}
