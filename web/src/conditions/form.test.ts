import { describe, expect, test } from 'vitest';

import { emptyConditions, hasConditions, toConditionsInput } from './form';

describe('toConditionsInput', () => {
  test.each([
    {
      name: '空白だけのキーワードと、照合する項目のないキーワードは送らない',
      form: {
        ...emptyConditions(),
        keywords: [
          { text: '  ', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
          { text: 'アニメ', regexp: false, caseSensitive: false, fields: [] },
          { text: ' 映画 ', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
        ],
      },
      want: [{ text: '映画', regexp: false, caseSensitive: false, fields: ['NAME'] }],
    },
  ])('$name', ({ form, want }) => {
    expect(toConditionsInput(form).keywords).toEqual(want);
  });

  test('曜日のない時間帯は送らない', () => {
    const form = {
      ...emptyConditions(),
      timeRanges: [
        { weekdays: [], startMinute: 0, durationMinutes: 60 },
        { weekdays: [0, 6], startMinute: 1380, durationMinutes: 120 },
      ],
    };
    expect(toConditionsInput(form).timeRanges).toEqual([
      { weekdays: [0, 6], startMinute: 1380, durationMinutes: 120 },
    ]);
  });
});

describe('hasConditions', () => {
  test.each([
    { name: '何もない', form: emptyConditions(), want: false },
    {
      name: '中身のないキーワードだけ',
      form: {
        ...emptyConditions(),
        keywords: [{ text: '', regexp: false, caseSensitive: false, fields: ['NAME' as const] }],
      },
      want: false,
    },
    { name: 'チャンネル種別', form: { ...emptyConditions(), channelTypes: ['BS'] }, want: true },
    { name: '最短の長さ', form: { ...emptyConditions(), minDurationMinutes: 30 }, want: true },
  ])('$name', ({ form, want }) => {
    expect(hasConditions(form)).toBe(want);
  });
});
