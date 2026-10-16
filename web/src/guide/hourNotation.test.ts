import { describe, expect, test } from 'vitest';

import { guideHour, guideTime } from './hourNotation';
import { broadcastDay } from './layout';

describe('guideTime', () => {
  // 2026-10-16 の放送日 (日本時間の 10/16 4:00〜10/17 4:00)。
  const at = (iso: string) => Date.parse(iso);
  const day = broadcastDay(at('2026-10-16T12:00:00+09:00')).from;

  test.each([
    { name: '朝は同じ', time: '2026-10-16T05:30:00+09:00', extended: true, want: '05:30' },
    { name: '夜も同じ', time: '2026-10-16T23:15:00+09:00', extended: true, want: '23:15' },
    {
      name: '深夜は 24 時以降で表す',
      time: '2026-10-17T01:05:00+09:00',
      extended: true,
      want: '25:05',
    },
    {
      name: '放送日の終わりの直前',
      time: '2026-10-17T03:59:00+09:00',
      extended: true,
      want: '27:59',
    },
    {
      name: '24 時間表記では、深夜は 0 時から',
      time: '2026-10-17T01:05:00+09:00',
      extended: false,
      want: '01:05',
    },
  ])('$name', ({ time, extended, want }) => {
    expect(guideTime(at(time), day, extended)).toBe(want);
  });

  test('目盛りの時は、28 時間表記で 4〜27 になる', () => {
    expect(guideHour(at('2026-10-16T04:00:00+09:00'), day, true)).toBe(4);
    expect(guideHour(at('2026-10-17T00:00:00+09:00'), day, true)).toBe(24);
    expect(guideHour(at('2026-10-17T00:00:00+09:00'), day, false)).toBe(0);
  });
});
