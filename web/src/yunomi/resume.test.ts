import { describe, expect, test } from 'vitest';

import { resumePosition } from './resume';

describe('resumePosition', () => {
  test.each([
    { name: '保存した位置がない', savedMs: null, want: 0 },
    { name: '少し見ただけなら、最初から', savedMs: 4_000, want: 0 },
    { name: '途中まで見ていたら、5 秒前から', savedMs: 754_500, want: 749.5 },
    { name: '終わりの近くまで見ていたら、最初から', savedMs: 1_795_000, want: 0 },
    { name: '5 秒戻すと下限より前になるなら、最初から', savedMs: 9_999, want: 0 },
    { name: '続きから再生する下限', savedMs: 10_000, want: 5 },
    { name: '続きから再生する上限', savedMs: 1_790_000, want: 1785 },
  ])('$name', ({ savedMs, want }) => {
    // 30 分の録画。
    expect(resumePosition(savedMs, 1800)).toBe(want);
  });
});
