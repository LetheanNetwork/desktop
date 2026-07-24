import { afterEach } from 'vitest';

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
