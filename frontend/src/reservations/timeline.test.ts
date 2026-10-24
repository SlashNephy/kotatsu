import { describe, expect, test } from 'vitest';

import { groupByDay, needsAttention } from './timeline';

const at = (time: string) => Date.parse(time);

describe('groupByDay', () => {
  test('日本時間の日付ごとにまとめる', () => {
    const items = [
      { id: 'a', startMs: at('2026-10-11T23:30:00+09:00') },
      // 日本時間では翌日。UTC では同じ日。
      { id: 'b', startMs: at('2026-10-12T00:30:00+09:00') },
      { id: 'c', startMs: at('2026-10-12T08:59:00+09:00') },
      { id: 'd', startMs: at('2026-10-12T09:00:00+09:00') },
    ];
    expect(groupByDay(items)).toEqual([
      { from: at('2026-10-11T00:00:00+09:00'), items: [items[0]] },
      { from: at('2026-10-12T00:00:00+09:00'), items: [items[1], items[2], items[3]] },
    ]);
  });

  test('同じ日の予約が離れて渡されても、1 つにまとめる', () => {
    const items = [
      { id: 'a', startMs: at('2026-10-12T10:00:00+09:00') },
      { id: 'b', startMs: at('2026-10-11T10:00:00+09:00') },
      { id: 'c', startMs: at('2026-10-12T11:00:00+09:00') },
    ];
    expect(groupByDay(items)).toEqual([
      { from: at('2026-10-11T00:00:00+09:00'), items: [items[1]] },
      { from: at('2026-10-12T00:00:00+09:00'), items: [items[0], items[2]] },
    ]);
  });

  test('予約がなければ、空', () => {
    expect(groupByDay([])).toEqual([]);
  });
});

describe('needsAttention', () => {
  test.each([
    { state: 'CONFLICT', enabled: true, want: true },
    { state: 'PARTIAL', enabled: true, want: true },
    { state: 'OK', enabled: true, want: false },
    { state: 'PENDING', enabled: true, want: false },
    // 無効にした予約は録画しないので、競合していても知らせない。
    { state: 'CONFLICT', enabled: false, want: false },
  ])('$state (enabled=$enabled)', ({ state, enabled, want }) => {
    expect(needsAttention(state, enabled)).toBe(want);
  });
});
