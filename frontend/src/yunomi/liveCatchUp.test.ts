import { describe, expect, test } from 'vitest';

import { liveLag, loadLowLatency } from './liveCatchUp';

describe('liveLag', () => {
  // ranges は、読み込んだ映像の範囲 (秒)。
  const video = (ranges: [number, number][], currentTime: number) => ({
    currentTime,
    buffered: {
      length: ranges.length,
      start: (i: number) => ranges[i]![0],
      end: (i: number) => ranges[i]![1],
    } as TimeRanges,
  });

  test.each([
    {
      name: '最後の範囲の末尾との差',
      ranges: [
        [0, 4],
        [5, 12.5],
      ] as [number, number][],
      currentTime: 10,
      want: 2.5,
    },
    {
      name: 'まだ読み込んでいなければ 0',
      ranges: [] as [number, number][],
      currentTime: 0,
      want: 0,
    },
  ])('$name', ({ ranges, currentTime, want }) => {
    expect(liveLag(video(ranges, currentTime))).toBe(want);
  });
});

describe('loadLowLatency', () => {
  test.each([
    { name: '覚えていなければ切る', stored: null, want: false },
    { name: '入れたことを覚えていれば入れる', stored: 'true', want: true },
  ])('$name', ({ stored, want }) => {
    expect(loadLowLatency({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ切る', () => {
    expect(
      loadLowLatency({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});
