import type { ChartData, ChartLabels } from './lthn-charts';
import './lthn-charts';

type ChartElement = HTMLElement & {
  data: ChartData | string;
  labels: ChartLabels | string;
  legend: boolean;
  updateComplete: Promise<unknown>;
};

afterEach(() => {
  document.body.replaceChildren();
});

describe('Lethean chart kit', () => {
  it('renders an accessible series summary and legend', async () => {
    const element = document.createElement('lthn-chart') as ChartElement;
    element.data = [{ name: 'Link', values: [2, 7, 4] }];
    element.labels = ['Mon', 'Tue', 'Wed'];
    element.legend = true;
    document.body.appendChild(element);
    await element.updateComplete;

    const plot = element.querySelector('svg[role="img"]');
    expect(plot?.getAttribute('aria-label')).toBe('line chart, series Link, peak 7');
    expect(element.textContent).toContain('Link');
    expect(element.textContent).toContain('Tue');
  });

  it('falls back to an accessible empty state for malformed data', async () => {
    const element = document.createElement('lthn-chart') as ChartElement;
    element.data = '{not-json';
    document.body.appendChild(element);
    await element.updateComplete;

    expect(element.querySelector('[role="img"]')?.getAttribute('aria-label')).toBe('No data yet.');
  });
});
