import { describe, expect, test } from 'vitest';

import { positionState } from './useMediaSession';

describe('positionState', () => {
  test.each([
    { name: 'ライブ', position: null, want: null },
    {
      name: '長さが分からない',
      position: { duration: 0, position: 0, playbackRate: 1 },
      want: null,
    },
    {
      name: '秒に丸める',
      position: { duration: 1800.6, position: 754.5, playbackRate: 1.5 },
      want: { duration: 1800, position: 754, playbackRate: 1.5 },
    },
    {
      name: '長さを超えた位置は、長さにする',
      position: { duration: 1800, position: 1800.9, playbackRate: 1 },
      want: { duration: 1800, position: 1800, playbackRate: 1 },
    },
    {
      name: '位置が分からなければ、始まりにする',
      position: { duration: 1800, position: Number.NaN, playbackRate: 1 },
      want: { duration: 1800, position: 0, playbackRate: 1 },
    },
  ])('$name', ({ position, want }) => {
    expect(positionState(position)).toEqual(want);
  });
});
