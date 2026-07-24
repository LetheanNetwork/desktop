import './lthn-core';

type LitTestElement = HTMLElement & {
  updateComplete: Promise<unknown>;
};

const mount = async <T extends LitTestElement>(
  tagName: string,
  configure?: (element: T) => void,
): Promise<T> => {
  const element = document.createElement(tagName) as T;
  configure?.(element);
  document.body.appendChild(element);
  await element.updateComplete;
  return element;
};

afterEach(() => {
  document.body.replaceChildren();
});

describe('Lethean core kit', () => {
  it('registers every core custom element', () => {
    const names = [
      'lthn-icon',
      'lthn-button',
      'lthn-badge',
      'lthn-card',
      'lthn-stat',
      'lthn-status-dot',
      'lthn-state-pill',
      'lthn-sparkline',
      'lthn-divider',
      'lthn-input',
      'lthn-field',
      'lthn-toggle',
      'lthn-brand-mark',
    ];

    names.forEach((name) => expect(customElements.get(name)).toBeDefined());
  });

  it('renders a loading button as busy and disabled', async () => {
    const element = await mount<LitTestElement & { loading: boolean; size: string }>(
      'lthn-button',
      (button) => {
        button.textContent = 'Save';
        button.loading = true;
        button.size = 'sm';
      },
    );

    const button = element.querySelector('button');

    expect(button?.disabled).toBe(true);
    expect(button?.getAttribute('aria-busy')).toBe('true');
    expect(button?.textContent).toContain('Save');
  });

  it('emits the next boolean value when a toggle changes', async () => {
    const element = await mount<LitTestElement & { on: boolean }>('lthn-toggle');
    const change = vi.fn();
    element.addEventListener('change', change);

    element.querySelector('button')?.click();
    await element.updateComplete;

    expect(element.on).toBe(true);
    expect(element.querySelector('[role="switch"]')?.getAttribute('aria-checked')).toBe('true');
    expect(change).toHaveBeenCalledOnce();
    expect((change.mock.calls[0]?.[0] as CustomEvent<boolean>).detail).toBe(true);
  });
});
