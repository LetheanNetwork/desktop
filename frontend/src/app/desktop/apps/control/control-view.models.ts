// SPDX-License-Identifier: EUPL-1.2

import type { DesktopDataState } from '../../desktop-data-state';
import type { ModelRuntimeModel, ModelRuntimeState } from '../../desktop-model-runtime.models';

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
  readonly state: ModelRuntimeState;
  readonly activeModelId: string;
  readonly availableModels: readonly ModelRuntimeModel[];
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

export interface ControlViewState {
  readonly dataState: DesktopDataState;
  readonly models: ControlModelsViewModel;
  readonly runs: ControlRunsViewModel;
  readonly power: ControlPowerViewModel;
  readonly system: ControlSystemViewModel;
}

export type ControlActionIntent = { readonly kind: 'new-run' };

export type ControlModelIntent =
  | { readonly kind: 'start' }
  | { readonly kind: 'load'; readonly modelId: string }
  | { readonly kind: 'unload' }
  | { readonly kind: 'restart' }
  | { readonly kind: 'stop' };
