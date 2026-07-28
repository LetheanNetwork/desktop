// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
} from '@angular/core';
import {
  type DesktopDataState,
  desktopDataStateLabel,
  desktopDataStateVariant,
} from './desktop-data-state';

@Component({
  selector: 'lthn-desktop-data-state',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <lthn-badge [attr.variant]="variant()" [attr.data-data-state]="state()">
      {{ label() }}
    </lthn-badge>
  `,
})
export class DesktopDataStateBadge {
  readonly state = input.required<DesktopDataState>();
  readonly label = computed(() => desktopDataStateLabel(this.state()));
  readonly variant = computed(() => desktopDataStateVariant(this.state()));
}
