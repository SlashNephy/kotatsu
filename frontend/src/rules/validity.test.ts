import { describe, expect, test } from 'vitest';

import {
  fromValidFromDate,
  fromValidUntilDate,
  outOfValidity,
  validFromDate,
  validUntilDate,
} from './validity';

// 表示のタイムゾーンは、既定の日本時間。
describe('有効期間の日付', () => {
  test('開始日は、その日の 0 時 (日本時間) から', () => {
    expect(fromValidFromDate('2026-10-01')).toBe('2026-09-30T15:00:00.000Z');
    expect(validFromDate('2026-09-30T15:00:00.000Z')).toBe('2026-10-01');
  });

  test('終了日は、その日を含め、翌日の 0 時 (日本時間) まで', () => {
    expect(fromValidUntilDate('2026-12-31')).toBe('2026-12-31T15:00:00.000Z');
    expect(validUntilDate('2026-12-31T15:00:00.000Z')).toBe('2026-12-31');
  });

  test('日付だけで入れる前の、時刻のある期間も、その時刻を含む日付で出す', () => {
    expect(validFromDate('2026-10-01T03:00:00.000Z')).toBe('2026-10-01');
    expect(validUntilDate('2026-10-01T03:00:00.000Z')).toBe('2026-10-01');
  });

  test.each(['', '2026-10', 'abc'])('%j は期間を区切らない', (date) => {
    expect(fromValidFromDate(date)).toBeNull();
    expect(fromValidUntilDate(date)).toBeNull();
  });

  test('区切らなければ空', () => {
    expect(validFromDate(null)).toBe('');
    expect(validUntilDate(null)).toBe('');
  });
});

describe('outOfValidity', () => {
  const from = '2026-09-30T15:00:00.000Z';
  const until = '2026-12-31T15:00:00.000Z';

  test.each([
    { name: '開始の前', now: '2026-09-30T14:59:59Z', want: true },
    { name: '開始のちょうど', now: '2026-09-30T15:00:00Z', want: false },
    { name: '終了の直前', now: '2026-12-31T14:59:59Z', want: false },
    { name: '終了のちょうど', now: '2026-12-31T15:00:00Z', want: true },
  ])('$name', ({ now, want }) => {
    expect(outOfValidity(from, until, Date.parse(now))).toBe(want);
  });

  test('区切らなければ、いつでも期間の中', () => {
    expect(outOfValidity(null, undefined, 0)).toBe(false);
  });
});
