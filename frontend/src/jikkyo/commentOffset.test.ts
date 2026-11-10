import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  formatCommentOffset,
  loadCommentOffset,
  stepCommentOffset,
  useCommentOffsetCommands,
} from './commentOffset';

const key = 'kotatsu.player.commentOffsetMs';

// storage は、getItem だけを持つ Storage の代わり。
const storage = (values: Record<string, string | undefined>) => ({
  getItem: (name: string) => values[name] ?? null,
});

describe('loadCommentOffset', () => {
  test.each([
    { name: 'なければ 0', values: {}, want: 0 },
    { name: '早める値を読む', values: { [key]: '-1500' }, want: -1500 },
    { name: '遅らせる値を読む', values: { [key]: '2000' }, want: 2000 },
    { name: '刻みにそろえる', values: { [key]: '700' }, want: 500 },
    { name: '刻みにそろえて 0 になる負の値は 0', values: { [key]: '-100' }, want: 0 },
    { name: '上限を超えていれば上限にする', values: { [key]: '90000' }, want: 30000 },
    { name: '下限を超えていれば下限にする', values: { [key]: '-90000' }, want: -30000 },
    { name: '数でなければ 0', values: { [key]: 'x' }, want: 0 },
  ])('$name', ({ values, want }) => {
    expect(loadCommentOffset(storage(values))).toBe(want);
  });

  test('読めないストレージなら 0', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
    };

    expect(loadCommentOffset(broken)).toBe(0);
    expect(loadCommentOffset(null)).toBe(0);
  });
});

describe('stepCommentOffset', () => {
  test.each([
    { name: '遅らせる', ms: 0, direction: 1, want: 500 },
    { name: '早める', ms: 0, direction: -1, want: -500 },
    { name: '0 をまたいで戻せる', ms: 500, direction: -1, want: 0 },
    { name: '上限で止まる', ms: 30000, direction: 1, want: 30000 },
    { name: '下限で止まる', ms: -30000, direction: -1, want: -30000 },
  ] as const)('$name', ({ ms, direction, want }) => {
    expect(stepCommentOffset(ms, direction)).toBe(want);
  });
});

describe('formatCommentOffset', () => {
  test.each([
    { ms: 0, want: '0.0' },
    { ms: 500, want: '+0.5' },
    { ms: -1500, want: '-1.5' },
  ])('$ms', ({ ms, want }) => {
    expect(formatCommentOffset(ms)).toBe(want);
  });
});

describe('useCommentOffsetCommands', () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  test.each([
    { name: '早める', command: 'commentOffsetEarlier', stored: '1000', want: 500 },
    { name: '遅らせる', command: 'commentOffsetLater', stored: '1000', want: 1500 },
    { name: '元に戻す', command: 'commentOffsetReset', stored: '1000', want: 0 },
  ] as const)('$name と、覚えて onChange に渡す', ({ command, stored, want }) => {
    window.localStorage.setItem(key, stored);
    const onChange = vi.fn();
    const { result } = renderHook(() => useCommentOffsetCommands(onChange));

    act(() => result.current[command]?.());

    expect(window.localStorage.getItem(key)).toBe(String(want));
    expect(onChange).toHaveBeenCalledExactlyOnceWith(want);
  });
});
