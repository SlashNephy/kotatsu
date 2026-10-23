import { describe, expect, test } from 'vitest';

import {
  broadcastTime,
  formatClock,
  formatDuration,
  parseStreamStart,
  playbackPosition,
  recordingPosition,
  skipJump,
} from './position';

describe('skipJump', () => {
  test.each([
    { name: '続けて進んだ', base: 1.4, last: 10, current: 10.25, want: 1.4 },
    { name: '2 倍速で 1 秒おきに進んだ', base: 1.4, last: 10, current: 12, want: 1.4 },
    // 途切れた区間の前後で、video の時刻が 126 秒飛んだ。飛んだ分を base に足し、位置に数えない。
    { name: '途切れた区間を越えた', base: 1.4, last: 10, current: 136, want: 127.4 },
    { name: '時刻が戻った', base: 1.4, last: 136, current: 10, want: -124.6 },
  ])('$name', ({ base, last, current, want }) => {
    expect(skipJump(base, last, current)).toBeCloseTo(want);
  });
});

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

describe('recordingPosition', () => {
  const from = '2026-10-14T12:00:00Z';
  const gap = { from: '2026-10-14T12:10:00Z', to: '2026-10-14T12:12:00Z' };
  test.each([
    {
      name: '途切れがなければ、始めた時刻からの秒',
      gaps: [],
      at: '2026-10-14T12:01:30Z',
      want: 90,
    },
    { name: '途切れる前の時刻', gaps: [gap], at: '2026-10-14T12:09:00Z', want: 9 * 60 },
    {
      name: '途切れた後の時刻は、区間の長さを引く',
      gaps: [gap],
      at: '2026-10-14T12:13:00Z',
      want: 11 * 60,
    },
    {
      name: '区間の中の時刻は、区間の始まりの位置',
      gaps: [gap],
      at: '2026-10-14T12:11:00Z',
      want: 10 * 60,
    },
    { name: '始めるより前は 0', gaps: [], at: '2026-10-14T11:59:00Z', want: 0 },
    {
      // 遅れて録り始めたときの冒頭の欠けは、基準 (書き始めた時刻) より前なので引かない。
      name: '基準より前の区間は引かない',
      gaps: [{ from: '2026-10-14T11:59:50Z', to: '2026-10-14T12:00:00Z' }],
      at: '2026-10-14T12:01:00Z',
      want: 60,
    },
  ])('$name', ({ gaps, at, want }) => {
    expect(recordingPosition(from, gaps, Date.parse(at))).toBe(want);
  });

  test('broadcastTime の逆になる', () => {
    const gaps = [gap, { from: '2026-10-14T12:20:00Z', to: '2026-10-14T12:21:00Z' }];

    expect(recordingPosition(from, gaps, broadcastTime(from, gaps, 25 * 60))).toBe(25 * 60);
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
