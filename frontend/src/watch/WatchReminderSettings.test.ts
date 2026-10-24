import { describe, expect, test } from 'vitest';

import { parseReminderMinutes } from './WatchReminderSettings';

describe('parseReminderMinutes', () => {
  test.each([
    { name: '数', value: 15, want: 15 },
    { name: '数の文字列', value: '15', want: 15 },
    { name: '放送の始まり', value: 0, want: 0 },
    { name: '上限', value: 60, want: 60 },
    { name: '上限を超える', value: 61, want: null },
    { name: '負の数', value: -1, want: null },
    { name: '小数', value: 1.5, want: null },
    { name: '空', value: '', want: null },
    { name: '数でない文字列', value: 'abc', want: null },
  ])('$name', ({ value, want }) => {
    expect(parseReminderMinutes(value)).toBe(want);
  });
});
