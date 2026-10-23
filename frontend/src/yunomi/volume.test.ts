import { describe, expect, test } from 'vitest';

import { defaultVolume, loadVolume, saveVolume, stepVolume } from './volume';

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

describe('loadVolume', () => {
  test.each([
    { name: '覚えた音量がない', stored: undefined, want: defaultVolume },
    {
      name: '覚えた音量を返す',
      stored: '{"volume":0.3,"muted":true}',
      want: { volume: 0.3, muted: true },
    },
    {
      name: '100% を超える音量も覚える (#453)',
      stored: '{"volume":1.5,"muted":false}',
      want: { volume: 1.5, muted: false },
    },
    {
      name: '上限 (200%) の外の音量は丸める',
      stored: '{"volume":4,"muted":false}',
      want: { volume: 2, muted: false },
    },
    { name: '壊れた JSON', stored: '{', want: defaultVolume },
    { name: '形の違う値', stored: '{"volume":"0.5","muted":false}', want: defaultVolume },
  ])('$name', ({ stored, want }) => {
    const storage = memoryStorage(stored === undefined ? {} : { 'kotatsu.player.volume': stored });

    expect(loadVolume(storage)).toEqual(want);
  });

  test('ストレージを使えなければ、既定の音量にする', () => {
    expect(loadVolume(null)).toEqual(defaultVolume);
    expect(
      loadVolume({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toEqual(defaultVolume);
  });
});

describe('saveVolume', () => {
  test('覚えた音量を、次に読める', () => {
    const storage = memoryStorage();
    saveVolume(storage, { volume: 0.42, muted: true });

    expect(loadVolume(storage)).toEqual({ volume: 0.42, muted: true });
  });

  test('書けなくても、例外を投げない', () => {
    expect(() =>
      saveVolume(
        {
          setItem: () => {
            throw new Error('quota');
          },
        },
        defaultVolume,
      ),
    ).not.toThrow();
  });
});

describe('stepVolume', () => {
  test.each([
    { name: '5% 上げる', volume: 0.5, delta: 0.05, want: 0.55 },
    { name: '100% を超えて上げる', volume: 1, delta: 0.05, want: 1.05 },
    { name: '200% より上げない', volume: 1.98, delta: 0.05, want: 2 },
    { name: '0 より下げない', volume: 0.02, delta: -0.05, want: 0 },
    { name: '誤差をためない', volume: 0.1 + 0.2, delta: 0.05, want: 0.35 },
  ])('$name', ({ volume, delta, want }) => {
    expect(stepVolume(volume, delta)).toBe(want);
  });
});
