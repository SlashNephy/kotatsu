import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  fullscreenLandscapeSupported,
  loadFullscreenLandscape,
  lockLandscape,
  saveFullscreenLandscape,
  unlockOrientation,
} from './landscapeLock';

function memoryStorage() {
  const items = new Map<string, string>();

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

// stubOrientation は、screen.orientation を差し替える。lock を渡さなければ、向きを固定できないブラウザにする。
function stubOrientation(orientation: { lock?: (o: string) => Promise<void>; unlock: () => void }) {
  vi.stubGlobal('screen', { orientation });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fullscreenLandscape (#1599)', () => {
  test('覚えた設定がなければ、横向きに固定する', () => {
    expect(loadFullscreenLandscape(memoryStorage())).toBe(true);
    expect(loadFullscreenLandscape(null)).toBe(true);
  });

  test('切った設定を覚える', () => {
    const storage = memoryStorage();
    saveFullscreenLandscape(storage, false);
    expect(loadFullscreenLandscape(storage)).toBe(false);
    saveFullscreenLandscape(storage, true);
    expect(loadFullscreenLandscape(storage)).toBe(true);
  });

  test('読めなければ、既定の固定する設定にする', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
    };
    expect(loadFullscreenLandscape(broken)).toBe(true);
  });

  test('横向きに固定し、外す', () => {
    const lock = vi.fn(() => Promise.resolve());
    const unlock = vi.fn();
    stubOrientation({ lock, unlock });

    expect(fullscreenLandscapeSupported()).toBe(true);
    lockLandscape();
    unlockOrientation();
    expect(lock).toHaveBeenCalledWith('landscape');
    expect(unlock).toHaveBeenCalledOnce();
  });

  test('固定を断られても、例外を投げない', async () => {
    const lock = vi.fn(() =>
      Promise.reject(new DOMException('not supported', 'NotSupportedError')),
    );
    stubOrientation({ lock, unlock: () => {} });

    expect(() => lockLandscape()).not.toThrow();
    await Promise.resolve();
    expect(lock).toHaveBeenCalledOnce();
  });

  test('向きを固定できないブラウザでは、設定を出さない', () => {
    stubOrientation({ unlock: () => {} });

    expect(fullscreenLandscapeSupported()).toBe(false);
  });
});
