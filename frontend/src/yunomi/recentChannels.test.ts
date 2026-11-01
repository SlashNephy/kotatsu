import { describe, expect, test } from 'vitest';

import {
  loadRecentChannels,
  maxRecentChannels,
  otherRecentChannels,
  recordChannel,
  saveRecentChannels,
} from './recentChannels';

function memoryStorage(initial: Record<string, string> = {}) {
  const items = new Map(Object.entries(initial));

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

describe('recordChannel', () => {
  test('見た局を先頭に置き、同じ局は 1 つにまとめる', () => {
    expect(recordChannel(['a', 'b', 'c'], 'b')).toEqual(['b', 'a', 'c']);
    expect(recordChannel([], 'a')).toEqual(['a']);
  });

  test('覚えておく数を超えたら、古いものから捨てる', () => {
    const full = Array.from({ length: maxRecentChannels }, (_, i) => `s${i}`);
    const next = recordChannel(full, 'new');
    expect(next).toHaveLength(maxRecentChannels);
    expect(next[0]).toBe('new');
    expect(next).not.toContain(`s${maxRecentChannels - 1}`);
  });
});

describe('otherRecentChannels', () => {
  test('今見ている局と、選べない局を除き、新しい順に返す', () => {
    const available = new Set(['a', 'b', 'c']);
    expect(otherRecentChannels(['a', 'gone', 'c', 'b'], 'a', available)).toEqual(['c', 'b']);
  });

  test('直前の局から戻ると、元の局が直前の局になる', () => {
    const available = new Set(['a', 'b', 'c']);
    // c を見たあと a に移った。
    const onA = recordChannel(recordChannel([], 'c'), 'a');
    expect(otherRecentChannels(onA, 'a', available)[0]).toBe('c');
    // 直前の局 (c) へ戻った。
    const onC = recordChannel(onA, 'c');
    expect(otherRecentChannels(onC, 'c', available)[0]).toBe('a');
  });
});

describe('loadRecentChannels', () => {
  test('覚えた履歴を、次に読める', () => {
    const storage = memoryStorage();
    saveRecentChannels(storage, ['a', 'b']);
    expect(loadRecentChannels(storage)).toEqual(['a', 'b']);
  });

  test.each([
    { name: '覚えていない', storage: memoryStorage() },
    { name: '壊れている', storage: memoryStorage({ 'kotatsu.live.recentChannels': '{' }) },
    { name: '配列でない', storage: memoryStorage({ 'kotatsu.live.recentChannels': '"a"' }) },
    { name: 'ストレージを使えない', storage: null },
  ])('$name なら、履歴を空にする', ({ storage }) => {
    expect(loadRecentChannels(storage)).toEqual([]);
  });

  test('文字列でない項目は捨てる', () => {
    const storage = memoryStorage({ 'kotatsu.live.recentChannels': '["a", 1, null, "b"]' });
    expect(loadRecentChannels(storage)).toEqual(['a', 'b']);
  });
});
