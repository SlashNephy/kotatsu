import { describe, expect, test } from 'vitest';

import { fitContain } from './capture';

describe('fitContain', () => {
  test.each([
    {
      name: '同じ比率なら、枠いっぱい',
      size: [1280, 720],
      want: { x: 0, y: 0, width: 1920, height: 1080 },
    },
    {
      name: '4:3 は、左右に黒い帯を残す',
      size: [640, 480],
      want: { x: 240, y: 0, width: 1440, height: 1080 },
    },
    {
      name: '大きさが分からなければ、描かない',
      size: [0, 0],
      want: { x: 0, y: 0, width: 0, height: 0 },
    },
  ])('$name', ({ size, want }) => {
    expect(fitContain(size[0] ?? 0, size[1] ?? 0, 1920, 1080)).toEqual(want);
  });
});
