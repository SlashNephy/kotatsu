import { describe, expect, test } from 'vitest';

import { readVideoStats } from './PlayerStats';

// ranges は、video.buffered と同じ形の TimeRanges を作る。
function ranges(list: [number, number][]): TimeRanges {
  return {
    length: list.length,
    start: (i: number) => list[i]![0],
    end: (i: number) => list[i]![1],
  };
}

describe('readVideoStats', () => {
  test.each([
    {
      name: '今の位置を含む範囲の、先に読み込んである秒数を数える',
      buffered: [
        [0, 5],
        [10, 30],
      ] as [number, number][],
      currentTime: 12,
      want: 18,
    },
    {
      name: '今の位置が読み込んだ範囲の外なら 0',
      buffered: [[0, 5]] as [number, number][],
      currentTime: 8,
      want: 0,
    },
    {
      name: '何も読み込んでいなければ 0',
      buffered: [] as [number, number][],
      currentTime: 0,
      want: 0,
    },
  ])('$name', ({ buffered, currentTime, want }) => {
    const stats = readVideoStats({
      videoWidth: 1440,
      videoHeight: 1080,
      currentTime,
      buffered: ranges(buffered),
      playbackRate: 1,
    });

    expect(stats.buffered).toBe(want);
  });

  test('フレームを数えられれば、捨てた数と全体の数を返す', () => {
    const stats = readVideoStats({
      videoWidth: 1920,
      videoHeight: 1080,
      currentTime: 0,
      buffered: ranges([]),
      playbackRate: 1.5,
      getVideoPlaybackQuality: () =>
        ({ droppedVideoFrames: 3, totalVideoFrames: 900, creationTime: 0 }) as VideoPlaybackQuality,
    });

    expect(stats).toEqual({
      width: 1920,
      height: 1080,
      dropped: 3,
      total: 900,
      buffered: 0,
      rate: 1.5,
    });
  });

  test('フレームを数えられないブラウザでは、null にする', () => {
    const stats = readVideoStats({
      videoWidth: 0,
      videoHeight: 0,
      currentTime: 0,
      buffered: ranges([]),
      playbackRate: 1,
    });

    expect([stats.dropped, stats.total]).toEqual([null, null]);
  });
});
