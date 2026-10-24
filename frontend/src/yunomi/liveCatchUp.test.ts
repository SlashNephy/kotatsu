import { describe, expect, test } from 'vitest';

import { catchUpRate, liveLag, loadLowLatency, nextPlaybackRate } from './liveCatchUp';

describe('nextPlaybackRate', () => {
  test.each([
    { name: '遅れが 3 秒を超えたら、追いつき始める', lag: 3.5, current: 1, want: catchUpRate },
    {
      name: '追いついている間は、1 秒まで縮むまで続ける',
      lag: 2,
      current: catchUpRate,
      want: catchUpRate,
    },
    { name: '1 秒まで縮んだら、元の速さに戻す', lag: 1, current: catchUpRate, want: 1 },
    { name: '追いついていなければ、3 秒までの遅れは許す', lag: 2, current: 1, want: 1 },
  ])('$name', ({ lag, current, want }) => {
    expect(nextPlaybackRate(lag, current)).toBe(want);
  });
});

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
