// SPDX-License-Identifier: EUPL-1.2

import type { SystemMonitorSnapshot } from '../../desktop-system-monitor.models';

export type TelemetryPanelProvenance = 'demo' | 'live';

export interface TelemetryMetricView {
  readonly label: string;
  readonly value: string;
  readonly unit: string;
  readonly history: readonly number[];
  readonly provenance: TelemetryPanelProvenance;
}

export interface TelemetryMetadataView {
  readonly label: string;
  readonly value: string;
}

export interface TelemetryViewData {
  readonly sample: SystemMonitorSnapshot;
  readonly primary: TelemetryMetricView;
  readonly secondary: TelemetryMetricView;
  readonly metadata: readonly TelemetryMetadataView[];
}
