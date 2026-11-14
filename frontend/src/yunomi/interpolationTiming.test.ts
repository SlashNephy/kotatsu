import { describe, expect, test } from 'vitest';

import { continues, framePeriod, mediaTimeAt, pickFrames } from './interpolationTiming';

describe('pickFrames', () => {
  const frames = [{ mediaTime: 1 }, { mediaTime: 1.04 }, { mediaTime: 1.08 }];

  test.each([
    { name: 'コマがなければ選ばない', frames: [], shown: 1, want: null },
    { name: '最初のコマより前なら最初のコマ', frames, shown: 0.5, want: { from: 0, to: 0, t: 0 } },
    {
      name: 'コマの時刻ちょうどなら、そのコマから始める',
      frames,
      shown: 1.04,
      want: { from: 1, to: 2, t: 0 },
    },
    {
      name: '2 コマの間なら、その間の位置',
      frames,
      shown: 1.01,
      want: { from: 0, to: 1, t: 0.25 },
    },
    { name: '最後のコマより後なら最後のコマ', frames, shown: 2, want: { from: 2, to: 2, t: 0 } },
    {
      name: '1 コマしかなければ、そのコマ',
      frames: [{ mediaTime: 1 }],
      shown: 1.5,
      want: { from: 0, to: 0, t: 0 },
    },
  ])('$name', ({ frames, shown, want }) => {
    const got = pickFrames(frames, shown);
    if (want === null) {
      expect(got).toBeNull();
    } else {
      expect(got?.from).toBe(want.from);
      expect(got?.to).toBe(want.to);
      expect(got?.t).toBeCloseTo(want.t);
    }
  });
});

describe('framePeriod', () => {
  test.each([
    { name: '2 コマなければ求めない', times: [1], want: null },
    { name: '等しい間隔なら、その間隔', times: [0, 1 / 30, 2 / 30, 3 / 30], want: 1 / 30 },
    // 取りこぼして間隔が倍になったところに引きずられない。
    {
      name: '取りこぼしがあっても、多い方の間隔',
      times: [0, 0.04, 0.08, 0.16, 0.2, 0.24],
      want: 0.04,
    },
    { name: '時刻が戻ったところは数えない', times: [0, 0.04, 0.02, 0.06, 0.1], want: 0.04 },
  ])('$name', ({ times, want }) => {
    const got = framePeriod(times);
    if (want === null) {
      expect(got).toBeNull();
    } else {
      expect(got).toBeCloseTo(want);
    }
  });
});

describe('mediaTimeAt', () => {
  test.each([
    { name: '等速なら、経った時間だけ進める', rate: 1, displayTime: 1_100, want: 10.1 },
    { name: '倍速なら、倍だけ進める', rate: 2, displayTime: 1_100, want: 10.2 },
    { name: '止まっていれば進めない', rate: 0, displayTime: 1_100, want: 10 },
  ])('$name', ({ rate, displayTime, want }) => {
    expect(mediaTimeAt({ mediaTime: 10, displayTime: 1_000, rate }, displayTime)).toBeCloseTo(want);
  });
});

describe('continues', () => {
  test.each([
    { name: '次のコマなら続き', previous: 1, next: 1.04, want: true },
    { name: '時刻が戻れば続きでない', previous: 1, next: 0.9, want: false },
    { name: '同じ時刻なら続きでない', previous: 1, next: 1, want: false },
    { name: '大きく飛べば続きでない', previous: 1, next: 2, want: false },
  ])('$name', ({ previous, next, want }) => {
    expect(continues(previous, next)).toBe(want);
  });
});
