import { describe, expect, test } from 'vitest';

import {
  canRetryImmediately,
  endedEarly,
  reconnectDelay,
  recordingsThatTookTuner,
} from './reconnect';

describe('canRetryImmediately', () => {
  const now = Date.parse('2026-10-27T20:00:00+09:00');
  test.each([
    { name: 'まだすぐつなぎ直していない', lastAt: null, want: true },
    { name: '30 秒より前にすぐつなぎ直した', lastAt: now - 30_000, want: true },
    // つないではすぐ切れるなら、知らせを出して間隔を延ばす。
    { name: '30 秒以内にすぐつなぎ直した', lastAt: now - 29_999, want: false },
  ])('$name', ({ lastAt, want }) => {
    expect(canRetryImmediately(lastAt, now)).toBe(want);
  });
});

describe('reconnectDelay', () => {
  test.each([
    { attempt: 0, want: 5 },
    { attempt: 1, want: 10 },
    { attempt: 2, want: 20 },
    { attempt: 3, want: 40 },
    { attempt: 4, want: 60 },
    // 60 秒より延ばさない。
    { attempt: 10, want: 60 },
  ])('$attempt 回目は $want 秒', ({ attempt, want }) => {
    expect(reconnectDelay(attempt)).toBe(want);
  });
});

describe('recordingsThatTookTuner', () => {
  const disconnectedAt = Date.parse('2026-10-11T20:00:00+09:00');
  const recordings = [
    { name: 'ちょうど始まった', recordedFrom: '2026-10-11T20:00:05+09:00' },
    { name: '開始の少し前に確保した', recordedFrom: '2026-10-11T19:58:30+09:00' },
    { name: '前から録画している', recordedFrom: '2026-10-11T19:30:00+09:00' },
  ];

  test('切れた時刻の前後に始まった録画だけを返す', () => {
    expect(recordingsThatTookTuner(recordings, disconnectedAt).map((r) => r.name)).toEqual([
      'ちょうど始まった',
      '開始の少し前に確保した',
    ]);
  });

  test('録画がなければ、空', () => {
    expect(recordingsThatTookTuner([], disconnectedAt)).toEqual([]);
  });
});

describe('endedEarly', () => {
  test.each([
    { name: '終わりまで受け取った', bufferedEnd: 1795, duration: 1800, want: false },
    { name: '終わりの少し手前まで受け取った', bufferedEnd: 1790, duration: 1800, want: false },
    { name: '途中で切れた', bufferedEnd: 600, duration: 1800, want: true },
  ])('$name', ({ bufferedEnd, duration, want }) => {
    expect(endedEarly(bufferedEnd, duration)).toBe(want);
  });
});
