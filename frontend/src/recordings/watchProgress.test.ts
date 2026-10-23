import { describe, expect, test } from 'vitest';

import { watchProgress } from './watchProgress';

describe('watchProgress', () => {
  test.each([
    { name: '再生していなければ null', savedMs: null, duration: 600, want: null },
    { name: '長さが分からなければ null', savedMs: 60_000, duration: 0, want: null },
    { name: '見た位置の割合にする', savedMs: 150_000, duration: 600, want: 0.25 },
    { name: '最初で止めたら 0', savedMs: 0, duration: 600, want: 0 },
    { name: '終わりの近くまで見ていたら 1', savedMs: 595_000, duration: 600, want: 1 },
    { name: '長さを超えた位置でも 1', savedMs: 700_000, duration: 600, want: 1 },
    { name: '負の位置は 0', savedMs: -1_000, duration: 600, want: 0 },
  ])('$name', ({ savedMs, duration, want }) => {
    expect(watchProgress(savedMs, duration)).toBe(want);
  });
});
