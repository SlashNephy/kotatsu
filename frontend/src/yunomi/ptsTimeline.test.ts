import { describe, expect, test } from 'vitest';

import { PtsTimeline, ptsWrap } from './ptsTimeline';

describe('PtsTimeline', () => {
  test.each([
    {
      name: '一周しなければ、そのまま返す',
      input: [1_000, 4_000, 3_000],
      want: [1_000, 4_000, 3_000],
    },
    {
      name: '一周したら、増え続ける数にする',
      input: [ptsWrap - 3_000, 6_000, 9_000],
      want: [ptsWrap - 3_000, ptsWrap + 6_000, ptsWrap + 9_000],
    },
    {
      // B フレームなど、少し前の時刻が後から届く。
      name: '一周の直後に、一周の前の時刻が届いても、前に戻す',
      input: [ptsWrap - 3_000, 6_000, ptsWrap - 1_000],
      want: [ptsWrap - 3_000, ptsWrap + 6_000, ptsWrap - 1_000],
    },
  ])('$name', ({ input, want }) => {
    const timeline = new PtsTimeline();

    expect(input.map((pts) => timeline.extend(pts))).toEqual(want);
  });

  test('reset したら、次の値から数え直す', () => {
    const timeline = new PtsTimeline();
    timeline.extend(ptsWrap - 3_000);
    timeline.extend(6_000);
    timeline.reset();

    expect(timeline.extend(6_000)).toBe(6_000);
  });
});
