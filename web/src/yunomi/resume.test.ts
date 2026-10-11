import { describe, expect, test } from 'vitest';

import { resumePosition } from './resume';

describe('resumePosition', () => {
  test.each([
    { name: '保存した位置がない', savedMs: null, want: 0 },
    { name: '少し見ただけなら、最初から', savedMs: 4_000, want: 0 },
    { name: '途中まで見ていたら、続きから', savedMs: 754_500, want: 754.5 },
    { name: '終わりの近くまで見ていたら、最初から', savedMs: 1_795_000, want: 0 },
  ])('$name', ({ savedMs, want }) => {
    // 30 分の録画。
    expect(resumePosition(savedMs, 1800)).toBe(want);
  });
});
