export interface ChartTooltipTheme {
  bg: string;
  border: string;
  fg: string;
}

export interface ChartTheme {
  palette: string[];
  grid: string;
  axis: string;
  axisLabel: string;
  font: string;
  fontSize: number;
  areaOpacity: number;
  barRadius: number;
  strokeWidth: number;
  tooltip: ChartTooltipTheme;
}

export const chartTheme: ChartTheme;

export function seriesColor(index: number): string;

export function resolveTheme(element?: Element): ChartTheme;
