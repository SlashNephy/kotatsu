import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { loadMiniPlayer, saveMiniPlayer, useMiniPlayer } from './miniPlayer';
import { browserStorage } from './volume';

// memoryStorage は、テストの localStorage の代わり。
function memoryStorage(): Pick<Storage, 'getItem' | 'setItem'> {
  const values = new Map<string, string>();

  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      values.set(key, value);
    },
  };
}

describe('miniPlayer', () => {
  test('既定では、小さなプレイヤーで続ける', () => {
    expect(loadMiniPlayer(memoryStorage())).toBe(true);
    expect(loadMiniPlayer(null)).toBe(true);
  });

  test('切ったら、続けない', () => {
    const storage = memoryStorage();
    saveMiniPlayer(storage, false);
    expect(loadMiniPlayer(storage)).toBe(false);
    saveMiniPlayer(storage, true);
    expect(loadMiniPlayer(storage)).toBe(true);
  });

  test('読めなければ、既定に戻る', () => {
    const broken: Pick<Storage, 'getItem'> = {
      getItem: () => {
        throw new Error('blocked');
      },
    };
    expect(loadMiniPlayer(broken)).toBe(true);
  });
});

describe('useMiniPlayer', () => {
  afterEach(() => {
    saveMiniPlayer(browserStorage(), true);
  });

  test('変えたら、同じタブの中の画面も追う', () => {
    const { result } = renderHook(() => useMiniPlayer());
    const other = renderHook(() => useMiniPlayer());
    expect(result.current[0]).toBe(true);

    act(() => {
      result.current[1](false);
    });
    expect(result.current[0]).toBe(false);
    expect(other.result.current[0]).toBe(false);
  });

  test('ほかのタブで変えたら、追う', () => {
    const { result } = renderHook(() => useMiniPlayer());

    act(() => {
      saveMiniPlayer(browserStorage(), false);
      window.dispatchEvent(new StorageEvent('storage'));
    });
    expect(result.current[0]).toBe(false);
  });
});
