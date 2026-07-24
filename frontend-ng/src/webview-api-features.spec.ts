import {
  WEBVIEW_API_FEATURE_FLAG,
  detectWebViewAPIs,
  installWebViewAPIFeatureFlag,
} from './webview-api-features';

describe('WebView API feature detection', () => {
  it('publishes every API from the Wails compatibility reference', () => {
    const features = detectWebViewAPIs({});

    expect(Object.keys(features)).toEqual([
      'storage',
      'network',
      'media',
      'graphics',
      'device',
      'workers',
      'performance',
      'security',
      'uiAndDOM',
      'notificationsAndMessaging',
      'navigationAndHistory',
      'sharingAndContent',
      'streamsAndEncoding',
      'payments',
      'extended',
      'css',
      'javascript',
    ]);
    expect(
      Object.fromEntries(
        Object.entries(features).map(([category, flags]) => [category, Object.keys(flags).length]),
      ),
    ).toEqual({
      storage: 9,
      network: 8,
      media: 13,
      graphics: 9,
      device: 19,
      workers: 4,
      performance: 11,
      security: 6,
      uiAndDOM: 16,
      notificationsAndMessaging: 5,
      navigationAndHistory: 5,
      sharingAndContent: 5,
      streamsAndEncoding: 12,
      payments: 2,
      extended: 18,
      css: 12,
      javascript: 21,
    });
  });

  it('distinguishes available APIs from absent APIs without requesting access', () => {
    class ElementFixture {
      animate(): void {}
      attachShadow(): void {}
      requestPointerLock(): void {}
    }
    class HTMLMediaElementFixture {
      remote = {};
    }

    const environment = {
      localStorage: {},
      fetch: () => Promise.resolve(),
      Worker: class {},
      BigInt,
      ArrayBuffer,
      crypto: { subtle: {} },
      navigator: {
        geolocation: {},
        mediaDevices: { getUserMedia: () => Promise.resolve() },
        storage: { getDirectory: () => Promise.resolve() },
      },
      document: {
        createElement: (tagName: string) => {
          if (tagName === 'canvas') {
            return {
              getContext: (context: string) => (context === '2d' ? {} : null),
            };
          }
          if (tagName === 'template') return { content: {} };
          if (tagName === 'div') return { draggable: false };
          return {};
        },
      },
      CSS: {
        supports: () => true,
      },
      Element: ElementFixture,
      HTMLMediaElement: HTMLMediaElementFixture,
    };

    const features = detectWebViewAPIs(environment);

    expect(features.storage.localStorage).toBe(true);
    expect(features.storage.originPrivateFileSystem).toBe(true);
    expect(features.network.fetch).toBe(true);
    expect(features.network.webTransport).toBe(false);
    expect(features.media.getUserMedia).toBe(true);
    expect(features.media.getDisplayMedia).toBe(false);
    expect(features.graphics.canvas2D).toBe(true);
    expect(features.graphics.webGL).toBe(false);
    expect(features.device.geolocation).toBe(true);
    expect(features.workers.webWorker).toBe(true);
    expect(features.security.webCrypto).toBe(true);
    expect(features.uiAndDOM.shadowDOM).toBe(true);
    expect(features.css.containerQueries).toBe(true);
    expect(features.javascript.bigInt).toBe(true);
  });

  it('fails closed when a WebView API getter or probe throws', () => {
    const environment: Record<string, unknown> = {
      document: {
        createElement: () => ({
          getContext: () => {
            throw new Error('graphics driver unavailable');
          },
        }),
      },
    };
    Object.defineProperty(environment, 'navigator', {
      get: () => {
        throw new Error('navigator unavailable');
      },
    });
    Object.defineProperty(environment, 'CSS', {
      get: () => {
        throw new Error('CSS namespace unavailable');
      },
    });

    expect(() => detectWebViewAPIs(environment)).not.toThrow();

    const features = detectWebViewAPIs(environment);
    expect(features.storage.storageAPI).toBe(false);
    expect(features.graphics.canvas2D).toBe(false);
    expect(features.device.geolocation).toBe(false);
    expect(features.css.supports).toBe(false);
  });

  it('installs one immutable, idempotent global feature flag', () => {
    const environment: Record<string, unknown> = { localStorage: {} };

    const installed = installWebViewAPIFeatureFlag(environment);
    const installedAgain = installWebViewAPIFeatureFlag(environment);

    expect(environment[WEBVIEW_API_FEATURE_FLAG]).toBe(installed);
    expect(installedAgain).toBe(installed);
    expect(Object.isFrozen(installed)).toBe(true);
    expect(Object.isFrozen(installed.storage)).toBe(true);
    expect(installed.storage.localStorage).toBe(true);
  });
});

describe('application bootstrap', () => {
  it('installs the WebView API flag before Angular starts', async () => {
    Reflect.deleteProperty(globalThis, WEBVIEW_API_FEATURE_FLAG);
    let featureFlagAtBootstrap: unknown;
    const bootstrapApplication = vi.fn(() => {
      featureFlagAtBootstrap = globalThis.__lthnWebViewAPIs;
      return Promise.resolve();
    });

    vi.resetModules();
    vi.doMock('@angular/platform-browser', () => ({ bootstrapApplication }));

    await import('./main');

    expect(bootstrapApplication).toHaveBeenCalledOnce();
    expect(featureFlagAtBootstrap).toBe(globalThis.__lthnWebViewAPIs);
    expect(featureFlagAtBootstrap).toBeDefined();
    expect(Object.isFrozen(featureFlagAtBootstrap)).toBe(true);

    vi.doUnmock('@angular/platform-browser');
  });
});
