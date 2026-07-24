import { DOCUMENT } from '@angular/common';
import { Injectable, InjectionToken, inject } from '@angular/core';

const createMemoryStorage = (): Storage => {
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

const createDesktopStorage = (): Storage => {
  const browserWindow = inject(DOCUMENT).defaultView;

  if (browserWindow) {
    try {
      return browserWindow.localStorage;
    } catch {
      // Fall through to an in-memory store for restricted webviews.
    }
  }

  return createMemoryStorage();
};

export const DESKTOP_STORAGE = new InjectionToken<Storage>('DESKTOP_STORAGE', {
  providedIn: 'root',
  factory: createDesktopStorage,
});

@Injectable({ providedIn: 'root' })
export class StorageService {
  private readonly storage = inject(DESKTOP_STORAGE);

  read<T = unknown>(key: string): T | null {
    try {
      return JSON.parse(this.storage.getItem(key) || 'null') as T | null;
    } catch {
      return null;
    }
  }

  write<T>(key: string, value: T): void {
    try {
      this.storage.setItem(key, JSON.stringify(value));
    } catch {}
  }
}
