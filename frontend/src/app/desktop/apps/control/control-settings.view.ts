// SPDX-License-Identifier: EUPL-1.2

import { ChangeDetectionStrategy, Component } from '@angular/core';
import { DesktopControlsPanelView } from '../../desktop-controls-panel.view';

@Component({
  selector: 'lthn-control-settings-view',
  standalone: true,
  imports: [DesktopControlsPanelView],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <lthn-desktop-controls-panel
      heading="Configuration"
      description="Desktop and native-host configuration from the shared reactive settings store."
      precedence="Defaults → File → Env → Set"
      help="Env overrides use CORE_CONFIG_*; environment values are never written back on Apply (dual-Viper)."
    />
  `,
})
export class ControlSettingsView {}
