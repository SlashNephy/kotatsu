import { describe, expect, test } from 'vitest';

import { tapSide } from './useDoubleTap';

describe('tapSide', () => {
  test.each([
    { name: '左の 3 分の 1', x: 100, want: 'left' },
    { name: '真ん中', x: 450, want: 'center' },
    { name: '右の 3 分の 1', x: 700, want: 'right' },
    { name: '境目は真ん中に含める', x: 300, want: 'center' },
  ])('$name', ({ x, want }) => {
    expect(tapSide(x, 900)).toBe(want);
  });
});
