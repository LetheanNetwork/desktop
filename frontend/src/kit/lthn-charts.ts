/* lthn-charts.ts — <lthn-chart>, the design system's graphing default.
 *
 * Line / area / bar in one element, on the token palette (foundations/chart-defaults.js).
 * Light-DOM (so tokens + fonts apply); data + labels are JSON attributes. For heavy
 * interactive charts, theme your charting lib with resolveTheme() from chart-defaults.
 *
 *   <lthn-chart type="area" data="[12,18,15,22,19,28]" labels='["Mon","Tue",…]'></lthn-chart>
 *   <lthn-chart type="line" data='[{"name":"Link","values":[…]}]' legend></lthn-chart>
 *   <lthn-chart type="bar" data="[4,9,7,12,10]"></lthn-chart>
 *
 * States: `loading` → skeleton · no data → the `empty-label` message.
 * A11y: the plot carries role="img" + an auto aria-label summarising type + series.
 */
import {
  LitElement,
  html,
  svg,
  nothing,
  type PropertyDeclarations,
  type SVGTemplateResult,
  type TemplateResult,
} from 'lit';
import { chartTheme, seriesColor } from '../foundations/chart-defaults.js';

export type ChartType = 'line' | 'area' | 'bar';
export type ChartPoint = number;
export interface ChartSeries {
  name?: string;
  values: readonly ChartPoint[];
  color?: string;
}
export type ChartData = readonly ChartPoint[] | readonly ChartSeries[];
export type ChartLabel = string | number;
export type ChartLabels = readonly ChartLabel[];
export interface ChartProperties {
  type?: ChartType;
  data?: ChartData | string;
  labels?: ChartLabels | string;
  height?: number;
  grid?: boolean;
  legend?: boolean;
  max?: number;
  loading?: boolean;
  emptyLabel?: string;
}

interface ResolvedChartSeries {
  name: string;
  values: readonly ChartPoint[];
  color: string;
}

type ChartScale = (value: number) => number;

const define = (name: string, cls: CustomElementConstructor): void => {
  if (!customElements.get(name)) customElements.define(name, cls);
};
const colorForSeries: (index: number) => string = seriesColor;
let uid: number = 0;

class LthnChart extends LitElement implements ChartProperties {
  static override properties: PropertyDeclarations = {
    type: {},
    data: {},
    labels: {},
    height: { type: Number },
    grid: { type: Boolean },
    legend: { type: Boolean },
    max: { type: Number },
    loading: { type: Boolean },
    emptyLabel: { attribute: 'empty-label' },
  };

  declare type: ChartType;
  declare data: ChartData | string;
  declare labels: ChartLabels | string;
  declare height: number;
  declare grid: boolean;
  declare legend: boolean;
  declare max: number;
  declare loading: boolean;
  declare emptyLabel: string;
  private readonly _id: string;

  constructor() {
    super();
    this.type = 'line';
    this.height = 220;
    this.grid = true;
    this.legend = false;
    this.loading = false;
    this.emptyLabel = $localize`:Chart empty state@@chart.emptyLabel:No data yet.`;
    this._id = 'lc' + ++uid;
  }
  protected override createRenderRoot(): HTMLElement | DocumentFragment {
    return this;
  }

  get series(): ResolvedChartSeries[] {
    let d: unknown;
    try {
      d = typeof this.data === 'string' ? JSON.parse(this.data) : this.data || [];
    } catch {
      d = [];
    }
    if (!Array.isArray(d) || !d.length) return [];
    if (typeof d[0] === 'number') {
      return [{ name: '', values: d as ChartPoint[], color: colorForSeries(0) }];
    }
    return (d as ChartSeries[]).map((s: ChartSeries, i: number): ResolvedChartSeries => ({
      name: s.name || '',
      values: s.values || [],
      color: s.color || colorForSeries(i),
    }));
  }

  get xlabels(): ChartLabels {
    try {
      return (
        typeof this.labels === 'string' ? JSON.parse(this.labels) : this.labels || []
      ) as ChartLabels;
    } catch {
      return [];
    }
  }

  protected override render(): TemplateResult {
    if (this.loading) {
      const loadingLabel = $localize`:Chart loading label@@chart.loadingLabel:Loading chart`;
      return html`<div
        aria-busy="true"
        aria-label=${loadingLabel}
        class="lthn-skeleton"
        style="width:100%;height:${this.height}px;border-radius:var(--r-md)"
      ></div>`;
    }
    const S = this.series;
    if (!S.length)
      return html`<div
        role="img"
        aria-label=${this.emptyLabel}
        style="width:100%;height:${this.height}px;display:grid;place-items:center;border:1px dashed var(--line-2);border-radius:var(--r-md);color:var(--fg-3);font-size:13px"
      >
        ${this.emptyLabel}
      </div>`;

    const W = 640,
      H = this.height,
      padL = 36,
      padR = 8,
      padT = 10,
      padB = this.xlabels.length ? 22 : 8;
    const iw = W - padL - padR,
      ih = H - padT - padB;
    const max =
      this.max ||
      Math.max(...S.flatMap((s: ResolvedChartSeries): readonly ChartPoint[] => s.values), 1) * 1.12;
    const n = Math.max(...S.map((s: ResolvedChartSeries): number => s.values.length), 2);
    const X: ChartScale = (i: number): number => padL + (n === 1 ? iw / 2 : (i / (n - 1)) * iw);
    const Y: ChartScale = (v: number): number => padT + ih - (v / max) * ih;
    const gridY: number[] = [0, 0.25, 0.5, 0.75, 1];
    const names = S.map((s: ResolvedChartSeries): string => s.name).filter(Boolean);
    const chartType = this.chartTypeLabel();
    const peak = Math.round(
      Math.max(...S.flatMap((s: ResolvedChartSeries): readonly ChartPoint[] => s.values)),
    );
    const label = names.length
      ? $localize`:Chart accessibility label with series@@chart.accessibleLabelWithSeries:${chartType}:chartType: chart, series ${names.join(', ')}:seriesNames:, peak ${peak}:peak:`
      : $localize`:Chart accessibility label@@chart.accessibleLabel:${chartType}:chartType: chart, peak ${peak}:peak:`;

    return html`<svg
        role="img"
        aria-label=${label}
        viewBox="0 0 ${W} ${H}"
        width="100%"
        height=${H}
        style="display:block;overflow:visible"
        font-family=${chartTheme.font}
      >
        <defs>
          ${S.map((s: ResolvedChartSeries, i: number): SVGTemplateResult => svg`<linearGradient id="${this._id}-${i}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color=${s.color} stop-opacity=${chartTheme.areaOpacity}></stop><stop offset="1" stop-color=${s.color} stop-opacity="0"></stop></linearGradient>`)}
        </defs>
        ${
        this.grid
          ? gridY.map((g: number): SVGTemplateResult => {
              const y = padT + ih - g * ih;
              return svg`<line x1=${padL} y1=${y} x2=${W - padR} y2=${y} stroke=${chartTheme.grid} stroke-width="1"></line>
          <text x=${padL - 8} y=${y + 3} text-anchor="end" font-size=${chartTheme.fontSize} fill=${chartTheme.axisLabel}>${Math.round(g * max)}</text>`;
            })
          : nothing
      }
        ${this.type === 'bar' ? this._bars(S, X, Y, ih, padT, n, iw) : this._lines(S, X, Y, padT, ih)}
        ${this.xlabels.length ? this.xlabels.map((lb: ChartLabel, i: number): SVGTemplateResult => svg`<text x=${X(i)} y=${H - 6} text-anchor="middle" font-size=${chartTheme.fontSize} fill=${chartTheme.axisLabel}>${lb}</text>`) : nothing}
      </svg>
      ${
      this.legend && names.length
        ? html`<div
            style="display:flex;flex-wrap:wrap;gap:14px;margin-top:10px;font-family:var(--font-mono);font-size:11px;color:var(--fg-3)"
          >
            ${S.map((s: ResolvedChartSeries): TemplateResult => html`<span style="display:inline-flex;align-items:center;gap:6px"><span style="width:9px;height:9px;border-radius:2px;background:${s.color}"></span>${s.name}</span>`)}
          </div>`
        : nothing
    }`;
  }

  private chartTypeLabel(): string {
    const labels: Record<ChartType, string> = {
      line: $localize`:Chart type@@chart.type.line:line`,
      area: $localize`:Chart type@@chart.type.area:area`,
      bar: $localize`:Chart type@@chart.type.bar:bar`,
    };
    return labels[this.type] ?? this.type;
  }

  private _lines(
    S: readonly ResolvedChartSeries[],
    X: ChartScale,
    Y: ChartScale,
    padT: number,
    ih: number,
  ): SVGTemplateResult[] {
    return S.map((s: ResolvedChartSeries, si: number): SVGTemplateResult => {
      const pts = s.values.map(
        (v: ChartPoint, i: number): string => `${X(i).toFixed(1)} ${Y(v).toFixed(1)}`,
      );
      const line = 'M ' + pts.join(' L ');
      const area = `${line} L ${X(s.values.length - 1).toFixed(1)} ${(padT + ih).toFixed(1)} L ${X(0).toFixed(1)} ${(padT + ih).toFixed(1)} Z`;
      return svg`${this.type === 'area' ? svg`<path d=${area} fill="url(#${this._id}-${si})"></path>` : nothing}
        <path d=${line} fill="none" stroke=${s.color} stroke-width=${chartTheme.strokeWidth} stroke-linejoin="round" stroke-linecap="round"></path>`;
    });
  }

  private _bars(
    S: readonly ResolvedChartSeries[],
    X: ChartScale,
    Y: ChartScale,
    ih: number,
    padT: number,
    n: number,
    iw: number,
  ): SVGTemplateResult[][] {
    const groups = S.length,
      slot = iw / n,
      bw = Math.max(3, (slot * 0.6) / groups);
    return S.map((s: ResolvedChartSeries, si: number): SVGTemplateResult[] =>
      s.values.map((v: ChartPoint, i: number): SVGTemplateResult => {
        const cx = X(i) - (groups * bw) / 2 + si * bw;
        const y = Y(v),
          h = padT + ih - y;
        return svg`<rect x=${cx.toFixed(1)} y=${y.toFixed(1)} width=${bw.toFixed(1)} height=${Math.max(0, h).toFixed(1)} rx=${chartTheme.barRadius} fill=${s.color}></rect>`;
      }),
    );
  }
}
define('lthn-chart', LthnChart);
