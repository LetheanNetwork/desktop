// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';

export type TelemetryPanelProvenance = 'demo' | 'live';
export type TelemetryConnectedState = 'live' | 'mixed';

export interface TelemetryDemoSeries {
  readonly throughput: readonly number[];
  readonly watts: readonly number[];
}

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
  readonly sample: ProcessTelemetry | null;
  readonly primary: TelemetryMetricView;
  readonly power: TelemetryMetricView;
  readonly metadata: readonly TelemetryMetadataView[];
}

export interface TelemetryLiveViewResult {
  readonly state: TelemetryConnectedState;
  readonly value: TelemetryViewData;
}
