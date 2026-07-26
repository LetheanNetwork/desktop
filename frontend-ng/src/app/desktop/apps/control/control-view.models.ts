// SPDX-License-Identifier: EUPL-1.2

import type { DesktopDataState } from '../../desktop-data-state';

export type ControlTableCell = string | number;
export type ControlTableRow = Readonly<Record<string, ControlTableCell>>;
export type ControlColumnType = 'num' | 'mono' | 'status';

export interface ControlTableColumn {
  readonly key: string;
  readonly label: string;
  readonly type?: ControlColumnType;
}

export interface ControlMetric {
  readonly value: string;
  readonly label: string;
}

export interface ControlChart {
  readonly title: string;
  readonly caption: string;
  readonly samples: readonly number[];
}

export interface ControlModelsViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly chart: ControlChart;
  readonly columns: readonly ControlTableColumn[];
  readonly rows: readonly ControlTableRow[];
}

export interface ControlRunsViewModel {
  readonly chart: ControlChart;
  readonly columns: readonly ControlTableColumn[];
  readonly rows: readonly ControlTableRow[];
}

export interface ControlPowerViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly samples: readonly number[];
}

export type ControlSystemTab = 'overview' | 'processes' | 'daemons';

export interface ControlSystemViewModel {
  readonly metrics: readonly ControlMetric[];
  readonly cpuSamples: readonly number[];
  readonly processColumns: readonly ControlTableColumn[];
  readonly processRows: readonly ControlTableRow[];
  readonly daemonColumns: readonly ControlTableColumn[];
  readonly daemonRows: readonly ControlTableRow[];
}

export interface ControlSettingRow {
  readonly key: string;
  readonly value: string;
  readonly source: string;
}

export interface ControlSettingGroup {
  readonly name: string;
  readonly rows: readonly ControlSettingRow[];
}

export interface ControlSettingFlag {
  readonly key: string;
  readonly on: boolean;
  readonly source: string;
}

export interface ControlSettingsViewModel {
  readonly groups: readonly ControlSettingGroup[];
  readonly flags: readonly ControlSettingFlag[];
}

export interface ControlViewState {
  readonly dataState: DesktopDataState;
  readonly models: ControlModelsViewModel;
  readonly runs: ControlRunsViewModel;
  readonly power: ControlPowerViewModel;
  readonly system: ControlSystemViewModel;
  readonly settings: ControlSettingsViewModel;
}

export type ControlActionIntent =
  | { readonly kind: 'load-model' }
  | { readonly kind: 'new-run' }
  | { readonly kind: 'commit-settings' };
