import { describe, expect, test } from 'vitest';

import { loadQuality, saveQuality } from './quality';

// memoryStorage は、localStorage の代わり。
function memoryStorage(initial: Record<string, string> = {}) {
  const items = new Map(Object.entries(initial));

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

const throwing = {
  getItem: () => {
    throw new Error('denied');
  },
  setItem: () => {
    throw new Error('denied');
  },
};

describe('loadQuality', () => {
  test.each([
    { name: '覚えていなければ undefined', storage: memoryStorage(), want: undefined },
    {
      name: '覚えた画質を返す',
      storage: memoryStorage({ 'kotatsu.player.quality': '1080p' }),
      want: '1080p',
    },
    {
      name: '空の値は、覚えていないものとみなす',
      storage: memoryStorage({ 'kotatsu.player.quality': '' }),
      want: undefined,
    },
    { name: '読めなければ undefined', storage: throwing, want: undefined },
    { name: 'localStorage がなければ undefined', storage: null, want: undefined },
  ])('$name', ({ storage, want }) => {
    expect(loadQuality(storage)).toBe(want);
  });
});

describe('saveQuality', () => {
  test('覚えた画質を、次に読める', () => {
    const storage = memoryStorage();

    saveQuality(storage, 'original');

    expect(loadQuality(storage)).toBe('original');
  });

  test('書けなくても、例外を投げない', () => {
    expect(() => saveQuality(throwing, '720p')).not.toThrow();
  });
});
