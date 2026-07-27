import { TestBed } from '@angular/core/testing';
import type { ModelRuntimeOperation } from '../../desktop-model-runtime.models';
import { ControlModelsView } from './control-models.view';
import { createDemoControlViewState } from './control-view-state';
import { ControlRunsView } from './control-runs.view';

describe('Control primary views', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('preserves the model tiles, chart, filters, and table while exposing stopped actions', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlModelsView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', {
      ...state.models,
      state: 'stopped',
      activeModelId: '',
      availableModels: [
        {
          id: 'model-0000000000000001',
          displayName: 'gemma-4-e2b',
          format: 'snapshot',
          loadable: true,
          loaded: false,
        },
      ],
    });
    fixture.componentRef.setInput('pending', null);
    const emitted = vi.fn();
    fixture.componentInstance.modelAction.subscribe(emitted);

    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Local models');
    expect(element.querySelectorAll('lthn-stat')).toHaveLength(4);
    expect(element.textContent).toContain('Running');
    expect(element.textContent).toContain('All');
    expect(element.querySelector('lthn-chart')).not.toBeNull();
    expect(
      JSON.parse(element.querySelector('lthn-datatable')?.getAttribute('rows') ?? '[]'),
    ).toHaveLength(6);

    expect(actionNames(element)).toEqual(['start']);
    element.querySelector<HTMLButtonElement>('[data-action="start"]')?.click();
    expect(emitted).toHaveBeenCalledWith({ kind: 'start' });
  });

  it.each([
    ['stopped', ['start']],
    ['starting', ['stop']],
    ['model-less', ['load', 'stop']],
    ['loading', ['stop']],
    ['ready', ['load', 'unload', 'restart', 'stop']],
    ['degraded', ['restart', 'stop']],
    ['failed', ['restart', 'stop']],
    ['stopping', []],
    ['unavailable', []],
  ] as const)('shows only valid %s lifecycle actions', async (runtimeState, expected) => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlModelsView);
    fixture.componentRef.setInput('dataState', state.dataState);
    fixture.componentRef.setInput('model', {
      ...state.models,
      state: runtimeState,
      activeModelId: '',
      availableModels: [
        {
          id: 'model-0000000000000001',
          displayName: 'gemma-4-e2b',
          format: 'snapshot',
          loadable: true,
          loaded: false,
        },
      ],
    });
    fixture.componentRef.setInput('pending', null);

    await fixture.whenStable();

    expect(actionNames(fixture.nativeElement as HTMLElement)).toEqual(expected);
  });

  it('routes the selected opaque model ID and disables every action while pending', async () => {
    const state = createDemoControlViewState();
    const fixture = TestBed.createComponent(ControlModelsView);
    fixture.componentRef.setInput('dataState', 'live');
    fixture.componentRef.setInput('model', {
      ...state.models,
      state: 'ready',
      activeModelId: '',
      availableModels: [
        {
          id: 'model-0000000000000001',
          displayName: 'gemma-4-e2b',
          format: 'snapshot',
          loadable: true,
          loaded: false,
        },
        {
          id: 'model-0000000000000002',
          displayName: 'not-loadable',
          format: 'snapshot',
          loadable: false,
          loaded: false,
        },
      ],
    });
    fixture.componentRef.setInput('pending', null);
    const emitted = vi.fn<(operation: ModelRuntimeOperation) => void>();
    fixture.componentInstance.modelAction.subscribe(emitted);
    await fixture.whenStable();

    const element = fixture.nativeElement as HTMLElement;
    const select = element.querySelector<HTMLSelectElement>('[aria-label="Model to load"]');
    expect(select?.options).toHaveLength(2);
    expect(select?.options[1].value).toBe('model-0000000000000001');
    if (!select) throw new Error('Expected the model selector.');
    select.value = 'model-0000000000000001';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="load"]')?.click();

    expect(emitted).toHaveBeenCalledWith({
      kind: 'load',
      modelId: 'model-0000000000000001',
    });

    fixture.componentRef.setInput('pending', {
      kind: 'load',
      modelId: 'model-0000000000000001',
    });
    await fixture.whenStable();
    expect(
      [...element.querySelectorAll<HTMLButtonElement>('[data-action]')].every(
        (button) => button.disabled,
      ),
    ).toBe(true);
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

function actionNames(element: HTMLElement): string[] {
  return [...element.querySelectorAll<HTMLElement>('[data-action]')].map(
    (action) => action.dataset['action'] ?? '',
  );
}
