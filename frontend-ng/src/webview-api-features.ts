/**
 * Synchronous WebView capability detection based on Wails' webview-api-check
 * example:
 * https://github.com/wailsapp/wails/tree/master/v3/examples/webview-api-check
 *
 * Checks only inspect or safely probe APIs; they never request a permission or
 * start a device operation.
 */

export const WEBVIEW_API_FEATURE_FLAG = '__lthnWebViewAPIs' as const;

export type WebViewAPIEnvironment = object;

function asObject(value: unknown): object | null {
  return (typeof value === 'object' && value !== null) || typeof value === 'function'
    ? value
    : null;
}

function read(source: unknown, key: PropertyKey): unknown {
  const object = asObject(source);
  if (!object) return undefined;

  try {
    return Reflect.get(object, key);
  } catch {
    return undefined;
  }
}

function has(source: unknown, key: PropertyKey): boolean {
  const object = asObject(source);
  if (!object) return false;

  try {
    return key in object;
  } catch {
    return false;
  }
}

function defined(source: unknown, key: PropertyKey): boolean {
  return read(source, key) !== undefined;
}

function prototypeOf(environment: object, constructorName: string): object | null {
  return asObject(read(read(environment, constructorName), 'prototype'));
}

function prototypeHas(environment: object, constructorName: string, propertyName: string): boolean {
  return has(prototypeOf(environment, constructorName), propertyName);
}

function elementHas(document: unknown, tagName: string, propertyName: string): boolean {
  const createElement = read(document, 'createElement');
  if (typeof createElement !== 'function') return false;

  try {
    return has(Reflect.apply(createElement, document, [tagName]), propertyName);
  } catch {
    return false;
  }
}

function canvasContext(document: unknown, ...contextNames: string[]): boolean {
  const createElement = read(document, 'createElement');
  if (typeof createElement !== 'function') return false;

  try {
    const canvas = Reflect.apply(createElement, document, ['canvas']);
    const getContext = read(canvas, 'getContext');
    if (typeof getContext !== 'function') return false;

    return contextNames.some(
      (contextName) => Reflect.apply(getContext, canvas, [contextName]) !== null,
    );
  } catch {
    return false;
  }
}

function callBoolean(source: unknown, methodName: string, ...args: unknown[]): boolean {
  const method = read(source, methodName);
  if (typeof method !== 'function') return false;

  try {
    return Boolean(Reflect.apply(method, source, args));
  } catch {
    return false;
  }
}

function supportsSyntax(environment: object, source: string): boolean {
  const functionConstructor = read(environment, 'Function');
  if (typeof functionConstructor !== 'function') return false;

  try {
    Reflect.construct(functionConstructor, [source]);
    return true;
  } catch {
    return false;
  }
}

function supportsRegExpFlag(environment: object, flag: string): boolean {
  const regexpConstructor = read(environment, 'RegExp');
  if (typeof regexpConstructor !== 'function') return false;

  try {
    Reflect.construct(regexpConstructor, ['.', flag]);
    return true;
  } catch {
    return false;
  }
}

function createWebViewAPIFeatures(environment: WebViewAPIEnvironment) {
  const webView = asObject(read(environment, 'window')) ?? environment;
  const navigator = read(webView, 'navigator');
  const document = read(webView, 'document');
  const screen = read(webView, 'screen');
  const history = read(webView, 'history');
  const performance = read(webView, 'performance');
  const css = read(webView, 'CSS');
  const storage = read(navigator, 'storage');
  const mediaDevices = read(navigator, 'mediaDevices');
  const clipboard = read(navigator, 'clipboard');
  const keyboard = read(navigator, 'keyboard');
  const scheduler = read(webView, 'scheduler');

  return Object.freeze({
    storage: Object.freeze({
      localStorage: defined(webView, 'localStorage'),
      sessionStorage: defined(webView, 'sessionStorage'),
      indexedDB: defined(webView, 'indexedDB'),
      cacheAPI: has(webView, 'caches'),
      cookieStore: has(webView, 'cookieStore'),
      storageAPI: defined(navigator, 'storage'),
      storageAccess: has(document, 'hasStorageAccess'),
      fileSystemAccess: has(webView, 'showOpenFilePicker'),
      originPrivateFileSystem: has(storage, 'getDirectory'),
    }),

    network: Object.freeze({
      fetch: defined(webView, 'fetch'),
      xmlHttpRequest: defined(webView, 'XMLHttpRequest'),
      webSocket: defined(webView, 'WebSocket'),
      eventSource: defined(webView, 'EventSource'),
      beacon: has(navigator, 'sendBeacon'),
      webTransport: defined(webView, 'WebTransport'),
      backgroundFetch: has(webView, 'BackgroundFetchManager'),
      backgroundSync: has(webView, 'SyncManager'),
    }),

    media: Object.freeze({
      webAudio: defined(webView, 'AudioContext') || defined(webView, 'webkitAudioContext'),
      mediaDevices: has(navigator, 'mediaDevices'),
      getUserMedia: has(mediaDevices, 'getUserMedia'),
      getDisplayMedia: has(mediaDevices, 'getDisplayMedia'),
      mediaRecorder: defined(webView, 'MediaRecorder'),
      mediaSession: has(navigator, 'mediaSession'),
      mediaCapabilities: has(navigator, 'mediaCapabilities'),
      mediaSource: defined(webView, 'MediaSource'),
      pictureInPicture: has(document, 'pictureInPictureEnabled'),
      audioWorklet: defined(webView, 'AudioWorkletNode'),
      speechRecognition:
        has(webView, 'SpeechRecognition') || has(webView, 'webkitSpeechRecognition'),
      speechSynthesis: has(webView, 'speechSynthesis'),
      encryptedMediaExtensions: has(webView, 'MediaKeys'),
    }),

    graphics: Object.freeze({
      canvas2D: canvasContext(document, '2d'),
      webGL: canvasContext(document, 'webgl', 'experimental-webgl'),
      webGL2: canvasContext(document, 'webgl2'),
      webGPU: has(navigator, 'gpu'),
      offscreenCanvas: defined(webView, 'OffscreenCanvas'),
      imageBitmap: defined(webView, 'createImageBitmap'),
      cssPainting: has(css, 'paintWorklet'),
      webAnimations: prototypeHas(webView, 'Element', 'animate'),
      viewTransitions: has(document, 'startViewTransition'),
    }),

    device: Object.freeze({
      geolocation: has(navigator, 'geolocation'),
      deviceOrientation: has(webView, 'DeviceOrientationEvent'),
      deviceMotion: has(webView, 'DeviceMotionEvent'),
      accelerometer: has(webView, 'Accelerometer'),
      gyroscope: has(webView, 'Gyroscope'),
      magnetometer: has(webView, 'Magnetometer'),
      ambientLightSensor: has(webView, 'AmbientLightSensor'),
      battery: has(navigator, 'getBattery'),
      deviceMemory: has(navigator, 'deviceMemory'),
      screenOrientation: has(screen, 'orientation'),
      wakeLock: has(navigator, 'wakeLock'),
      vibration: has(navigator, 'vibrate'),
      midi: has(navigator, 'requestMIDIAccess'),
      serial: has(navigator, 'serial'),
      hid: has(navigator, 'hid'),
      usb: has(navigator, 'usb'),
      nfc: has(webView, 'NDEFReader'),
      bluetooth: has(navigator, 'bluetooth'),
      gamepad: has(navigator, 'getGamepads'),
    }),

    workers: Object.freeze({
      webWorker: defined(webView, 'Worker'),
      sharedWorker: defined(webView, 'SharedWorker'),
      serviceWorker: has(navigator, 'serviceWorker'),
      worklets: defined(webView, 'Worklet'),
    }),

    performance: Object.freeze({
      performanceAPI: defined(webView, 'performance'),
      performanceObserver: defined(webView, 'PerformanceObserver'),
      navigationTiming: defined(webView, 'PerformanceNavigationTiming'),
      resourceTiming: defined(webView, 'PerformanceResourceTiming'),
      userTiming: has(performance, 'mark') && has(performance, 'measure'),
      longTasks: defined(webView, 'PerformanceLongTaskTiming'),
      intersectionObserver: defined(webView, 'IntersectionObserver'),
      resizeObserver: defined(webView, 'ResizeObserver'),
      mutationObserver: defined(webView, 'MutationObserver'),
      reporting: defined(webView, 'ReportingObserver'),
      computePressure: has(webView, 'PressureObserver'),
    }),

    security: Object.freeze({
      webCrypto: defined(webView, 'crypto') && has(read(webView, 'crypto'), 'subtle'),
      credentials: has(navigator, 'credentials'),
      webAuthentication: defined(webView, 'PublicKeyCredential'),
      permissions: has(navigator, 'permissions'),
      trustedTypes: has(webView, 'trustedTypes'),
      contentSecurityPolicy: defined(webView, 'SecurityPolicyViolationEvent'),
    }),

    uiAndDOM: Object.freeze({
      customElements: has(webView, 'customElements'),
      shadowDOM: prototypeHas(webView, 'Element', 'attachShadow'),
      htmlTemplates: elementHas(document, 'template', 'content'),
      pointerEvents: has(webView, 'PointerEvent'),
      touchEvents:
        has(webView, 'ontouchstart') ||
        (typeof read(navigator, 'maxTouchPoints') === 'number' &&
          Number(read(navigator, 'maxTouchPoints')) > 0),
      pointerLock: prototypeHas(webView, 'Element', 'requestPointerLock'),
      fullscreen: has(document, 'fullscreenEnabled') || has(document, 'webkitFullscreenEnabled'),
      selection: defined(webView, 'Selection'),
      clipboard: has(navigator, 'clipboard'),
      clipboardRead: has(clipboard, 'read'),
      clipboardWrite: has(clipboard, 'write'),
      dragAndDrop: elementHas(document, 'div', 'draggable'),
      editContext: has(webView, 'EditContext'),
      virtualKeyboard: has(navigator, 'virtualKeyboard'),
      popover: prototypeHas(webView, 'HTMLElement', 'popover'),
      dialog: defined(webView, 'HTMLDialogElement'),
    }),

    notificationsAndMessaging: Object.freeze({
      notifications: has(webView, 'Notification'),
      push: has(webView, 'PushManager'),
      channelMessaging: defined(webView, 'MessageChannel'),
      broadcastChannel: defined(webView, 'BroadcastChannel'),
      postMessage: has(webView, 'postMessage'),
    }),

    navigationAndHistory: Object.freeze({
      history: has(history, 'pushState'),
      navigation: has(webView, 'navigation'),
      url: defined(webView, 'URL'),
      urlSearchParams: defined(webView, 'URLSearchParams'),
      urlPattern: defined(webView, 'URLPattern'),
    }),

    sharingAndContent: Object.freeze({
      share: has(navigator, 'share'),
      shareTarget: has(navigator, 'share') && has(navigator, 'canShare'),
      badging: has(navigator, 'setAppBadge'),
      contentIndex: has(webView, 'ContentIndex'),
      contactPicker: has(navigator, 'contacts'),
    }),

    streamsAndEncoding: Object.freeze({
      readableStream: defined(webView, 'ReadableStream'),
      writableStream: defined(webView, 'WritableStream'),
      transformStream: defined(webView, 'TransformStream'),
      compressionStreams: defined(webView, 'CompressionStream'),
      textEncoderDecoder: defined(webView, 'TextEncoder') && defined(webView, 'TextDecoder'),
      textEncoderStream: defined(webView, 'TextEncoderStream'),
      blob: defined(webView, 'Blob'),
      file: defined(webView, 'File') && defined(webView, 'FileReader'),
      fileReader: defined(webView, 'FileReader'),
      arrayBuffer: defined(webView, 'ArrayBuffer'),
      dataView: defined(webView, 'DataView'),
      typedArrays: defined(webView, 'Uint8Array'),
    }),

    payments: Object.freeze({
      paymentRequest: has(webView, 'PaymentRequest'),
      paymentHandler: has(webView, 'PaymentManager'),
    }),

    extended: Object.freeze({
      webXR: has(navigator, 'xr'),
      presentation: has(navigator, 'presentation'),
      remotePlayback: prototypeHas(webView, 'HTMLMediaElement', 'remote'),
      windowManagement: has(webView, 'getScreenDetails'),
      documentPictureInPicture: has(webView, 'documentPictureInPicture'),
      eyeDropper: has(webView, 'EyeDropper'),
      fileHandling: has(webView, 'launchQueue'),
      launchHandler: has(webView, 'LaunchParams'),
      idleDetection: has(webView, 'IdleDetector'),
      keyboardLock: has(keyboard, 'lock'),
      localFontAccess: has(webView, 'queryLocalFonts'),
      screenCapture: has(mediaDevices, 'getDisplayMedia'),
      scheduler: has(webView, 'scheduler'),
      taskAttribution: defined(webView, 'TaskAttributionTiming'),
      videoCodecs: defined(webView, 'VideoEncoder'),
      audioCodecs: defined(webView, 'AudioEncoder'),
      webLocks: has(navigator, 'locks'),
      prioritisedTaskScheduling: has(scheduler, 'postTask'),
    }),

    css: Object.freeze({
      cssOM: defined(webView, 'CSSStyleSheet'),
      constructableStyleSheets: has(document, 'adoptedStyleSheets'),
      typedOM: prototypeHas(webView, 'Element', 'attributeStyleMap'),
      propertiesAndValues: has(css, 'registerProperty'),
      supports: has(css, 'supports'),
      fontLoading: has(document, 'fonts'),
      containerQueries: callBoolean(css, 'supports', 'container-type', 'inline-size'),
      cascadeLayers: callBoolean(css, 'supports', '@layer test { }'),
      subgrid: callBoolean(css, 'supports', 'grid-template-columns', 'subgrid'),
      hasSelector: callBoolean(css, 'supports', 'selector(:has(a))'),
      colourMix: callBoolean(css, 'supports', 'color', 'color-mix(in srgb, red, blue)'),
      scrollDrivenAnimations: callBoolean(css, 'supports', 'animation-timeline', 'scroll()'),
    }),

    javascript: Object.freeze({
      esModules: elementHas(document, 'script', 'noModule'),
      importMaps: callBoolean(read(webView, 'HTMLScriptElement'), 'supports', 'importmap'),
      dynamicImport: supportsSyntax(
        webView,
        'return import("data:text/javascript,export default 1")',
      ),
      topLevelAwait: true,
      weakRef: defined(webView, 'WeakRef'),
      finalizationRegistry: defined(webView, 'FinalizationRegistry'),
      bigInt: defined(webView, 'BigInt'),
      globalThis: true,
      optionalChaining: supportsSyntax(webView, 'const value = null?.property'),
      nullishCoalescing: supportsSyntax(webView, 'const value = null ?? "default"'),
      privateClassFields: supportsSyntax(webView, 'class Feature { #value = true }'),
      staticClassBlocks: supportsSyntax(webView, 'class Feature { static {} }'),
      temporal: defined(webView, 'Temporal'),
      iteratorHelpers: defined(webView, 'Iterator') && has(read(webView, 'Iterator'), 'from'),
      arrayAt: prototypeHas(webView, 'Array', 'at'),
      objectHasOwn: has(read(webView, 'Object'), 'hasOwn'),
      structuredClone: defined(webView, 'structuredClone'),
      atomicsWaitAsync: has(read(webView, 'Atomics'), 'waitAsync'),
      arrayFromAsync: has(read(webView, 'Array'), 'fromAsync'),
      promiseWithResolvers: has(read(webView, 'Promise'), 'withResolvers'),
      regexpUnicodeSets: supportsRegExpFlag(webView, 'v'),
    }),
  });
}

export type WebViewAPIFeatureFlags = ReturnType<typeof createWebViewAPIFeatures>;

export function detectWebViewAPIs(
  environment: WebViewAPIEnvironment = globalThis,
): WebViewAPIFeatureFlags {
  return createWebViewAPIFeatures(environment);
}

const installedFlags = new WeakSet<object>();

export function installWebViewAPIFeatureFlag(
  environment: WebViewAPIEnvironment = globalThis,
): WebViewAPIFeatureFlags {
  const existing = read(environment, WEBVIEW_API_FEATURE_FLAG);
  const existingObject = asObject(existing);
  if (existingObject && installedFlags.has(existingObject)) {
    return existingObject as WebViewAPIFeatureFlags;
  }

  const features = detectWebViewAPIs(environment);
  Object.defineProperty(environment, WEBVIEW_API_FEATURE_FLAG, {
    configurable: true,
    enumerable: true,
    value: features,
    writable: false,
  });
  installedFlags.add(features);
  return features;
}

declare global {
  // eslint-disable-next-line no-var
  var __lthnWebViewAPIs: WebViewAPIFeatureFlags | undefined;

  interface Window {
    readonly __lthnWebViewAPIs?: WebViewAPIFeatureFlags;
  }
}
