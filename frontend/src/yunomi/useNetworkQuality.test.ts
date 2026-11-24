import { describe, expect, test } from 'vitest';

import { loadedToEnd } from './useNetworkQuality';

// ranges は、[始まり, 終わり] の並びから、読み込んである範囲を作る。
const ranges = (spans: [number, number][]) => ({
  length: spans.length,
  start: (index: number) => spans[index]![0],
  end: (index: number) => spans[index]![1],
});

describe('loadedToEnd', () => {
  test.each([
    {
      name: '今の位置の範囲が長さの終わりまであれば、読み込んである',
      spans: [[0, 600]] as [number, number][],
      currentTime: 595,
      duration: 600,
      want: true,
    },
    {
      // HLS のプレイリストの長さの和と、実際の端のずれ。
      name: '終わりの手前の誤差の内なら、読み込んである',
      spans: [[0, 599.7]] as [number, number][],
      currentTime: 595,
      duration: 600,
      want: true,
    },
    {
      name: '最後のセグメント (2 秒) が届いていなければ、読み込んでいない',
      spans: [[0, 598]] as [number, number][],
      currentTime: 590,
      duration: 600,
      want: false,
    },
    {
      name: '終わりまでの範囲が今の位置と離れていれば、読み込んでいない',
      spans: [
        [100, 120],
        [590, 600],
      ] as [number, number][],
      currentTime: 110,
      duration: 600,
      want: false,
    },
    {
      // 音声のほうが長く、映像の範囲の端を今の位置が越えたとき。
      name: '今の位置が終わりの誤差の内なら、範囲の外でも読み込んである',
      spans: [[0, 599.7]] as [number, number][],
      currentTime: 599.8,
      duration: 600,
      want: true,
    },
    {
      name: '長さが 0 なら、読み込んでいない',
      spans: [[0, 0]] as [number, number][],
      currentTime: 0,
      duration: 0,
      want: false,
    },
    {
      name: '長さが決まっていなければ (ライブ)、読み込んでいない',
      spans: [[0, 600]] as [number, number][],
      currentTime: 595,
      duration: Number.POSITIVE_INFINITY,
      want: false,
    },
    {
      name: '長さが分からなければ (NaN)、読み込んでいない',
      spans: [] as [number, number][],
      currentTime: 0,
      duration: Number.NaN,
      want: false,
    },
  ])('$name', ({ spans, currentTime, duration, want }) => {
    expect(loadedToEnd({ buffered: ranges(spans), currentTime, duration })).toBe(want);
  });
});
