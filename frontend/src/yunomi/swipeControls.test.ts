import { describe, expect, test } from 'vitest';

import { loadSwipeControls, saveSwipeControls } from './swipeControls';

function memoryStorage() {
  const items = new Map<string, string>();

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

describe('swipeControls (#1598)', () => {
  test('覚えた設定がなければ、スワイプを受け付けない', () => {
    expect(loadSwipeControls(memoryStorage())).toBe(false);
    expect(loadSwipeControls(null)).toBe(false);
  });

  test('入れた設定を覚える', () => {
    const storage = memoryStorage();
    saveSwipeControls(storage, true);
    expect(loadSwipeControls(storage)).toBe(true);
  });
});
