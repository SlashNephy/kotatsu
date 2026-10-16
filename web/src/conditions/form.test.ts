import { describe, expect, test } from 'vitest';

import {
  conditionsFromProgram,
  type ConditionsData,
  emptyConditions,
  fromConditions,
  hasConditions,
  toConditionsInput,
} from './form';

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
    { name: '最短の長さ', form: { ...emptyConditions(), minDurationSeconds: 1800 }, want: true },
  ])('$name', ({ form, want }) => {
    expect(hasConditions(form)).toBe(want);
  });
});

describe('fromConditions と toConditionsInput', () => {
  const data = (min: number | null, max: number | null): ConditionsData => ({
    keywords: [],
    keywordMatch: 'ALL',
    excludeKeywords: [],
    channelTypes: [],
    services: [],
    genres: [],
    excludeGenres: [],
    timeRanges: [],
    minDurationSeconds: min,
    maxDurationSeconds: max,
    freeOnly: false,
  });

  test.each([
    { name: '分で割り切れない長さ', min: 90, max: 5430 },
    { name: '分で割り切れる長さ', min: 1200, max: 7200 },
    { name: '長さの条件がない', min: null, max: null },
  ])('$name は、読んでそのまま書き戻しても変わらない', ({ min, max }) => {
    const input = toConditionsInput(fromConditions(data(min, max)));
    expect([input.minDurationSeconds, input.maxDurationSeconds]).toEqual([min, max]);
  });
});

describe('conditionsFromProgram', () => {
  test('番組名の囲み文字を外してキーワードにし、サービスで絞る', () => {
    expect(conditionsFromProgram('🈑ニュース7🈞', 'U2VydmljZToxMjM')).toEqual({
      ...emptyConditions(),
      keywords: [{ text: 'ニュース7', regexp: false, caseSensitive: false, fields: ['NAME'] }],
      services: ['U2VydmljZToxMjM'],
    });
  });

  test('囲み文字だけの番組名では、キーワードを入れない', () => {
    expect(conditionsFromProgram('🈑', 'U2VydmljZToxMjM').keywords).toEqual([]);
  });
});
