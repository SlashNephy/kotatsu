import { describe, expect, test } from 'vitest';

import { formatDuration, playbackPosition } from './position';

describe('playbackPosition', () => {
  test.each([
    { name: '最初から読んだ', offset: 0, base: 0, current: 12.5, want: 12.5 },
    { name: 'シークした位置から読んだ', offset: 90, base: 0, current: 3, want: 93 },
    {
      name: '変換した映像の時刻が 0 から始まらない',
      offset: 90,
      base: 1.4,
      current: 4.4,
      want: 93,
    },
    {
      name: '読み込み直後に時刻が base より前を指しても、開始位置より前にしない',
      offset: 90,
      base: 1.4,
      current: 1,
      want: 90,
    },
  ])('$name', ({ offset, base, current, want }) => {
    expect(playbackPosition(offset, base, current)).toBeCloseTo(want);
  });
});

describe('formatDuration', () => {
  test.each([
    { seconds: 0, want: '0:00' },
    { seconds: 59.9, want: '0:59' },
    { seconds: 61, want: '1:01' },
    { seconds: 3600 + 2 * 60 + 3, want: '1:02:03' },
    { seconds: -5, want: '0:00' },
  ])('$seconds 秒', ({ seconds, want }) => {
    expect(formatDuration(seconds)).toBe(want);
  });
});
