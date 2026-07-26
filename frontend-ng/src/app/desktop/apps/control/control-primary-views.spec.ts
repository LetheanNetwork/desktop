import { TestBed } from '@angular/core/testing';
import { ControlModelsView } from './control-models.view';
import { createDemoControlViewState } from './control-view-state';
import { ControlRunsView } from './control-runs.view';

describe('Control primary views', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('renders Models from its typed input and emits Load model', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlModelsView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.models);
    const emitted = vi.fn();
    fixture.componentInstance.loadModel.subscribe(emitted);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Local models');
    expect(element.querySelectorAll('lthn-stat')).toHaveLength(4);
    expect(
      JSON.parse(element.querySelector('lthn-datatable')?.getAttribute('rows') ?? '[]'),
    ).toHaveLength(6);

    element.querySelector<HTMLButtonElement>('button.nbtn')?.click();
    expect(emitted).toHaveBeenCalledOnce();
  });

  it('renders Runs from its typed input and emits New run', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlRunsView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', state.runs);
    const emitted = vi.fn();
    fixture.componentInstance.newRun.subscribe(emitted);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Benchmark runs');
    expect(element.querySelector('lthn-chart')?.getAttribute('data')).toBe(
      '[34.2,112.5,88.1,41.7]',
    );

    element.querySelector<HTMLButtonElement>('button.nbtn')?.click();
    expect(emitted).toHaveBeenCalledOnce();
  });
});
