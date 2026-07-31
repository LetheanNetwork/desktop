// SPDX-License-Identifier: EUPL-1.2

import { TELEMETRY } from '../../desktop.data';
import type { ControlViewState } from './control-view.models';

export const CONTROL_DEMO_VIEW_STATE = {
  dataState: 'demo',
  models: {
    state: 'ready',
    activeModelId: '',
    availableModels: [],
    metrics: [
      {
        value: '34.2',
        label: $localize`:Tokens per second unit@@unit.tokensPerSecond:tok/s`,
      },
      {
        value: '18.4 GB',
        label: $localize`:Video memory metric@@metric.vram:VRAM`,
      },
      {
        value: '128',
        label: $localize`:Requests per minute metric@@metric.requestsPerMinute:Req / min`,
      },
      {
        value: '6d 4h',
        label: $localize`:Uptime metric@@metric.uptime:Uptime`,
      },
    ],
    chart: {
      title: $localize`:Throughput chart title@@control.models.throughputChart:Throughput · last hour`,
      caption: $localize`:Throughput chart peak@@control.models.throughputPeak:peak 41.8 tok/s`,
      samples: TELEMETRY.throughput,
    },
    columns: [
      {
        key: 'name',
        label: $localize`:Model table column@@control.column.model:Model`,
      },
      {
        key: 'rate',
        label: $localize`:Model table column@@control.column.tokensPerSecond:tok/s`,
        type: 'num',
      },
      {
        key: 'vram',
        label: $localize`:Model table column@@control.column.vram:VRAM`,
        type: 'num',
      },
      {
        key: 'region',
        label: $localize`:Model table column@@control.column.region:Region`,
        type: 'mono',
      },
      {
        key: 'status',
        label: $localize`:Model table column@@control.column.state:State`,
        type: 'status',
      },
    ],
    rows: [
      {
        name: 'llama-3.1-70b',
        rate: 34.2,
        vram: 18.4,
        region: 'eu-west-2',
        status: 'running',
      },
      {
        name: 'mistral-small',
        rate: 0,
        vram: 6.1,
        region: 'eu-west-2',
        status: 'active',
      },
      {
        name: 'gemma-2-27b',
        rate: 0,
        vram: 0,
        region: 'eu-west-1',
        status: 'idle',
      },
      {
        name: 'qwen-2.5-coder',
        rate: 0,
        vram: 0,
        region: 'eu-west-2',
        status: 'idle',
      },
      {
        name: 'phi-3-mini',
        rate: 0,
        vram: 2.3,
        region: 'eu-west-1',
        status: 'stalled',
      },
      {
        name: 'llama-3.1-8b',
        rate: 112.5,
        vram: 5.8,
        region: 'eu-west-2',
        status: 'running',
      },
    ],
  },
  runs: {
    chart: {
      title: $localize`:Benchmark chart title@@control.runs.chart:tok/s by run`,
      caption: $localize`:Benchmark chart range@@control.runs.lastFour:last 4`,
      samples: [34.2, 112.5, 88.1, 41.7],
    },
    columns: [
      {
        key: 'run',
        label: $localize`:Run table column@@control.column.run:Run`,
        type: 'mono',
      },
      {
        key: 'model',
        label: $localize`:Run table column@@control.column.model:Model`,
      },
      {
        key: 'ctx',
        label: $localize`:Run table column@@control.column.context:ctx`,
        type: 'num',
      },
      {
        key: 'toks',
        label: $localize`:Run table column@@control.column.tokensPerSecond:tok/s`,
        type: 'num',
      },
      {
        key: 'when',
        label: $localize`:Run table column@@control.column.when:When`,
        type: 'mono',
      },
    ],
    rows: [
      {
        run: '#0424',
        model: 'llama-3.1-70b',
        ctx: 8_192,
        toks: 34.2,
        when: '09:12',
      },
      {
        run: '#0423',
        model: 'llama-3.1-8b',
        ctx: 8_192,
        toks: 112.5,
        when: '08:40',
      },
      {
        run: '#0422',
        model: 'mistral-small',
        ctx: 4_096,
        toks: 88.1,
        when: '08:05',
      },
      {
        run: '#0421',
        model: 'gemma-2-27b',
        ctx: 8_192,
        toks: 41.7,
        when: 'Tue',
      },
    ],
  },
  power: {
    metrics: [
      {
        value: '196 W',
        label: $localize`:Daily average power metric@@control.power.dailyAverage:24h avg`,
      },
      {
        value: '4.7 kWh',
        label: $localize`:Daily total power metric@@control.power.dailyTotal:24h total`,
      },
      {
        value: '210 W',
        label: $localize`:Daily peak power metric@@control.power.peakToday:Peak today`,
      },
    ],
    samples: TELEMETRY.watts,
  },
  system: {
    metrics: [
      {
        value: '34%',
        label: $localize`:CPU metric@@metric.cpu:CPU`,
      },
      {
        value: '18.4 / 32 GB',
        label: $localize`:Memory metric@@metric.memory:Memory`,
      },
      {
        value: '6',
        label: $localize`:Process count metric@@metric.processes:Processes`,
      },
      {
        value: '4',
        label: $localize`:Daemon count metric@@metric.daemons:Daemons`,
      },
    ],
    cpuSamples: TELEMETRY.throughput,
    processColumns: [
      {
        key: 'proc',
        label: $localize`:Process table column@@control.column.process:Process`,
        type: 'mono',
      },
      {
        key: 'pid',
        label: $localize`:Process table column@@control.column.pid:PID`,
        type: 'num',
      },
      {
        key: 'cpu',
        label: $localize`:Process table column@@control.column.cpu:CPU%`,
        type: 'num',
      },
      {
        key: 'mem',
        label: $localize`:Process table column@@control.column.memory:MEM`,
        type: 'num',
      },
      {
        key: 'state',
        label: $localize`:Process table column@@control.column.state:State`,
        type: 'status',
      },
    ],
    processRows: [
      {
        proc: 'lthn-runner',
        pid: 4_821,
        cpu: 31.2,
        mem: 18.4,
        state: 'running',
      },
      {
        proc: 'llama-server',
        pid: 4_830,
        cpu: 2.1,
        mem: 6.1,
        state: 'running',
      },
      {
        proc: 'lethernet',
        pid: 5_102,
        cpu: 0.8,
        mem: 0.4,
        state: 'active',
      },
      {
        proc: 'go-config',
        pid: 5_110,
        cpu: 0,
        mem: 0.1,
        state: 'idle',
      },
      {
        proc: 'forge-watch',
        pid: 5_140,
        cpu: 0,
        mem: 0.2,
        state: 'idle',
      },
      {
        proc: 'mistral-server',
        pid: 5_201,
        cpu: 0,
        mem: 6.1,
        state: 'stalled',
      },
    ],
    daemonColumns: [
      {
        key: 'code',
        label: $localize`:Daemon table column@@control.column.daemon:Daemon`,
        type: 'mono',
      },
      {
        key: 'daemon',
        label: $localize`:Daemon table column@@control.column.kind:Kind`,
      },
      {
        key: 'pid',
        label: $localize`:Daemon table column@@control.column.pid:PID`,
        type: 'num',
      },
      {
        key: 'health',
        label: $localize`:Daemon table column@@control.column.health:Health`,
        type: 'status',
      },
      {
        key: 'project',
        label: $localize`:Daemon table column@@control.column.project:Project`,
      },
    ],
    daemonRows: [
      {
        code: 'lthn',
        daemon: 'inference',
        pid: 4_821,
        health: 'running',
        project: 'lethean',
      },
      {
        code: 'go-pool',
        daemon: 'worker',
        pid: 5_201,
        health: 'active',
        project: 'mining',
      },
      {
        code: 'go-net',
        daemon: 'mesh',
        pid: 5_102,
        health: 'running',
        project: 'lethernet',
      },
      {
        code: 'go-cfg',
        daemon: 'config',
        pid: 5_110,
        health: 'idle',
        project: 'core',
      },
    ],
  },
} satisfies ControlViewState;
