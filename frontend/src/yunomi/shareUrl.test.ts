import { describe, expect, test } from 'vitest';

import {
  parseStartSeconds,
  positionUrl,
  timeshiftShareSeconds,
  timeshiftStartPosition,
} from './shareUrl';

describe('parseStartSeconds', () => {
  test.each([
    { name: '数', value: 90, want: 90 },
    { name: '数の文字列', value: '90', want: 90 },
    { name: '0 は、最初からの指定として受け取る', value: 0, want: 0 },
    { name: '負の数は受け取らない', value: -1, want: undefined },
    { name: '数でない文字列は受け取らない', value: 'abc', want: undefined },
    { name: '指定がない', value: undefined, want: undefined },
    { name: '空の t は、指定がないものとして扱う', value: '', want: undefined },
    { name: '空白だけの t も、指定がないものとして扱う', value: '  ', want: undefined },
    { name: 'null は受け取らない', value: null, want: undefined },
    { name: '真偽値は受け取らない', value: true, want: undefined },
  ])('$name', ({ value, want }) => {
    expect(parseStartSeconds(value)).toBe(want);
  });
});

describe('positionUrl', () => {
  test.each([
    {
      name: '秒の端数を切り捨てて付ける',
      href: 'https://kotatsu.test/recordings/abc/play',
      seconds: 90.9,
      want: 'https://kotatsu.test/recordings/abc/play?t=90',
    },
    {
      name: '今の t を置き換える',
      href: 'https://kotatsu.test/recordings/abc/play?t=10',
      seconds: 90,
      want: 'https://kotatsu.test/recordings/abc/play?t=90',
    },
    {
      name: 'ほかのクエリは残し、fragment は外す',
      href: 'https://kotatsu.test/timeshift/s/play?program=2026-11-10T10%3A00%3A00%2B09%3A00#x',
      seconds: 5,
      want: 'https://kotatsu.test/timeshift/s/play?program=2026-11-10T10%3A00%3A00%2B09%3A00&t=5',
    },
  ])('$name', ({ href, seconds, want }) => {
    expect(positionUrl(href, seconds)).toBe(want);
  });
});

describe('タイムシフトの位置と URL の t', () => {
  const startAt = '2026-11-10T01:00:00Z';

  test('頭のチャンクを捨てて起点が動いても、t は放送の同じ場面を指す', () => {
    const before = { startAt, playableFrom: startAt, gaps: [] };
    const after = { startAt, playableFrom: '2026-11-10T01:10:00Z', gaps: [] };

    const seconds = timeshiftShareSeconds(before, 1200);

    expect(seconds).toBe(1200);
    expect(timeshiftStartPosition(after, seconds)).toBe(600);
  });

  test('途切れた区間の後ろの位置は、区間の長さを含めて表し、元の位置に戻せる', () => {
    const range = {
      startAt,
      playableFrom: startAt,
      gaps: [{ from: '2026-11-10T01:05:00Z', to: '2026-11-10T01:06:00Z' }],
    };

    const seconds = timeshiftShareSeconds(range, 600);

    expect(seconds).toBe(660);
    expect(timeshiftStartPosition(range, seconds)).toBe(600);
  });

  test('もう捨てた所を指していたら、再生できる範囲の始まりから', () => {
    const range = { startAt, playableFrom: '2026-11-10T01:10:00Z', gaps: [] };

    expect(timeshiftStartPosition(range, 60)).toBe(0);
  });
});
