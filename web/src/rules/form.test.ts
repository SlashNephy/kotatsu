import { describe, expect, test } from 'vitest';

import { emptyRuleForm, fromRule, hasConditions, toInput, type RuleData } from './form';

// imported は、EPGStation から取り込んだような、画面で編集しない項目まで埋まったルール。
const imported: RuleData = {
  name: '名探偵',
  memo: 'メモ',
  enabled: true,
  conditions: {
    keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
    excludeKeywords: [
      { text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
    ],
    channelTypes: ['GR'],
    services: [{ id: 'U2VydmljZTox' }],
    genres: [{ lv1: 7, lv2: 0 }],
    excludeGenres: [{ lv1: 0, lv2: null }],
    timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
    minDurationSeconds: 1200,
    maxDurationSeconds: 7200,
    freeOnly: true,
  },
  settings: {
    poolId: 'main',
    fileNameTemplate: '{{.Name}}',
    startMarginSeconds: 5,
    endMarginSeconds: 10,
    priority: 3,
  },
  validFrom: '2026-10-01T00:00:00Z',
  validUntil: null,
  avoidDuplicates: true,
  duplicateWindowDays: 30,
  followEventRelay: false,
};

describe('fromRule と toInput', () => {
  test('画面で編集しない項目も、保存し直したときに失わない', () => {
    expect(toInput(fromRule(imported))).toEqual({
      name: '名探偵',
      memo: 'メモ',
      enabled: true,
      conditions: {
        keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
        excludeKeywords: [
          { text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
        ],
        channelTypes: ['GR'],
        services: ['U2VydmljZTox'],
        genres: [{ lv1: 7, lv2: 0 }],
        excludeGenres: [{ lv1: 0, lv2: null }],
        timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
        minDurationSeconds: 1200,
        maxDurationSeconds: 7200,
        freeOnly: true,
      },
      settings: {
        poolId: 'main',
        fileNameTemplate: '{{.Name}}',
        startMarginSeconds: 5,
        endMarginSeconds: 10,
        priority: 3,
      },
      validFrom: '2026-10-01T00:00:00Z',
      validUntil: null,
      avoidDuplicates: true,
      duplicateWindowDays: 30,
      followEventRelay: false,
    });
  });
});

describe('toInput', () => {
  test.each([
    {
      name: '空白だけのキーワードと、照合する項目のないキーワードは送らない',
      form: {
        ...emptyRuleForm(),
        keywords: [
          { text: '  ', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
          { text: 'アニメ', regexp: false, caseSensitive: false, fields: [] },
          { text: ' 映画 ', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
        ],
      },
      want: [{ text: '映画', regexp: false, caseSensitive: false, fields: ['NAME'] }],
    },
  ])('$name', ({ form, want }) => {
    expect(toInput(form).conditions.keywords).toEqual(want);
  });

  test('曜日のない時間帯は送らない', () => {
    const form = {
      ...emptyRuleForm(),
      timeRanges: [
        { weekdays: [], startMinute: 0, durationMinutes: 60 },
        { weekdays: [0, 6], startMinute: 1380, durationMinutes: 120 },
      ],
    };
    expect(toInput(form).conditions.timeRanges).toEqual([
      { weekdays: [0, 6], startMinute: 1380, durationMinutes: 120 },
    ]);
  });

  test('重複を避けないときは、期間を送らない', () => {
    const form = { ...emptyRuleForm(), avoidDuplicates: false, duplicateWindowDays: 30 };
    expect(toInput(form).duplicateWindowDays).toBeNull();
  });
});

describe('hasConditions', () => {
  test.each([
    { name: '何もない', form: emptyRuleForm(), want: false },
    {
      name: '中身のないキーワードだけ',
      form: {
        ...emptyRuleForm(),
        keywords: [{ text: '', regexp: false, caseSensitive: false, fields: ['NAME' as const] }],
      },
      want: false,
    },
    { name: 'チャンネル種別', form: { ...emptyRuleForm(), channelTypes: ['BS'] }, want: true },
    { name: '最短の長さ', form: { ...emptyRuleForm(), minDurationMinutes: 30 }, want: true },
  ])('$name', ({ form, want }) => {
    expect(hasConditions(form)).toBe(want);
  });
});
