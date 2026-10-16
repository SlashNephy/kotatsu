import { describe, expect, test } from 'vitest';

import {
  broadcastTime,
  formatClock,
  formatDuration,
  parseStreamStart,
  playbackPosition,
} from './position';

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

describe('broadcastTime', () => {
  const from = '2026-10-14T12:00:00Z';
  const gap = { from: '2026-10-14T12:10:00Z', to: '2026-10-14T12:12:00Z' };
  test.each([
    {
      name: '途切れがなければ、始めた時刻に足す',
      gaps: [],
      seconds: 90,
      want: '2026-10-14T12:01:30Z',
    },
    { name: '途切れる前の位置', gaps: [gap], seconds: 9 * 60, want: '2026-10-14T12:09:00Z' },
    {
      name: '途切れた後の位置は、区間の長さを足す',
      gaps: [gap],
      seconds: 11 * 60,
      want: '2026-10-14T12:13:00Z',
    },
    {
      name: '区間の並びによらない',
      gaps: [{ from: '2026-10-14T12:20:00Z', to: '2026-10-14T12:21:00Z' }, gap],
      seconds: 20 * 60,
      want: '2026-10-14T12:23:00Z',
    },
  ])('$name', ({ gaps, seconds, want }) => {
    expect(broadcastTime(from, gaps, seconds)).toBe(Date.parse(want));
  });
});

describe('formatClock', () => {
  test('日本時間の HH:MM:SS で表す', () => {
    expect(formatClock(Date.parse('2026-10-14T12:01:30Z'))).toBe('21:01:30');
  });
});

describe('parseStreamStart', () => {
  test.each([
    { value: '2.002', want: 2.002 },
    { value: '0.000', want: 0 },
    { value: null, want: null },
    { value: '', want: null },
    { value: 'abc', want: null },
    { value: '-1', want: null },
  ])('$value は $want', ({ value, want }) => {
    expect(parseStreamStart(value)).toBe(want);
  });
});
