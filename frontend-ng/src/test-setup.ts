import { afterAll, afterEach } from 'vitest';

const createTestStorage = (): Storage => {
  const values = new Map<string, string>();

  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => {
      values.delete(key);
    },
    setItem: (key, value) => values.set(key, value),
  };
};

const testLocalStorage = createTestStorage();
const testSessionStorage = createTestStorage();
const defineStorage = (
  target: object,
  name: 'localStorage' | 'sessionStorage',
  storage: Storage,
): void => {
  Object.defineProperty(target, name, {
    configurable: true,
    value: storage,
  });
};

defineStorage(globalThis, 'localStorage', testLocalStorage);
defineStorage(globalThis, 'sessionStorage', testSessionStorage);
if (window !== globalThis) {
  defineStorage(window, 'localStorage', testLocalStorage);
  defineStorage(window, 'sessionStorage', testSessionStorage);
}

// @wailsio/runtime installs a short-lived drag-environment polling interval at
// module import. A fast Vitest file can finish before its first tick, after
// which the package callback would reference a torn-down jsdom `window`.
// Track browser-owned intervals so each test environment closes them before
// teardown. Production uses the unmodified browser timer implementation.
const browserSetInterval = window.setInterval.bind(window);
const browserClearInterval = window.clearInterval.bind(window);
const browserIntervals = new Set<number>();
window.setInterval = ((handler: TimerHandler, timeout?: number, ...args: unknown[]): number => {
  const id = Reflect.apply(browserSetInterval, window, [handler, timeout, ...args]) as number;
  browserIntervals.add(id);
  return id;
}) as typeof window.setInterval;
window.clearInterval = ((id?: number): void => {
  if (id !== undefined) browserIntervals.delete(id);
  browserClearInterval(id);
}) as typeof window.clearInterval;

// jsdom's canvas stub logs a "not implemented" error before returning null.
// Components already treat a null context as the non-WebGL fallback, so keep
// that browser contract without polluting otherwise-green test output.
Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', {
  configurable: true,
  value: () => null,
});

(globalThis as typeof globalThis & { litIssuedWarnings?: Set<string> }).litIssuedWarnings = new Set(
  ['dev-mode'],
);

afterEach(() => {
  testLocalStorage.clear();
  testSessionStorage.clear();
});

afterAll(() => {
  for (const id of browserIntervals) browserClearInterval(id);
  browserIntervals.clear();
});
