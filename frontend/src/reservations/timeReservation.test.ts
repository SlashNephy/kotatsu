import { describe, expect, test } from 'vitest';

import { fromInputValue, toInputValue, validateRange } from './timeReservation';

describe('toInputValue と fromInputValue', () => {
  test.each([
    { name: '日本時間の昼', iso: '2026-10-17T03:00:00Z', value: '2026-10-17T12:00' },
    { name: '日本時間で日付が進む深夜', iso: '2026-10-17T16:30:00Z', value: '2026-10-18T01:30' },
  ])('$name', ({ iso, value }) => {
    expect(toInputValue(Date.parse(iso))).toBe(value);
    expect(fromInputValue(value)).toBe(Date.parse(iso));
  });

  test.each(['', '2026-10-17', '2026-10-17T12:00:30', 'abc'])('%j は読めない', (value) => {
    expect(fromInputValue(value)).toBeNull();
  });
});

describe('validateRange', () => {
  const start = Date.parse('2026-10-17T03:00:00Z');
  const hour = 3_600_000;

  test.each([
    { name: '正しい時間帯', end: start + hour, want: null },
    { name: 'ちょうど 24 時間', end: start + 24 * hour, want: null },
    { name: '24 時間を超える', end: start + 24 * hour + 60_000, want: 'tooLong' },
    { name: '終わりが始まりと同じ', end: start, want: 'order' },
    { name: '終わりが始まりより前', end: start - hour, want: 'order' },
    { name: '終わりがない', end: null, want: 'missing' },
  ])('$name', ({ end, want }) => {
    expect(validateRange(start, end)).toBe(want);
  });
});
