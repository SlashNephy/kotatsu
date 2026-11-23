import { describe, expect, test } from 'vitest';

import {
  conditionsFromProgram,
  type ConditionsData,
  emptyConditions,
  fromConditions,
  hasConditions,
  hasRuleTargets,
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
    { name: '含める囲み文字', form: { ...emptyConditions(), marks: ['新'] }, want: true },
    { name: '除外する囲み文字', form: { ...emptyConditions(), excludeMarks: ['再'] }, want: true },
    {
      name: '除外するサービス',
      form: { ...emptyConditions(), excludeServices: ['U2VydmljZTox'] },
      want: true,
    },
  ])('$name', ({ form, want }) => {
    expect(hasConditions(form)).toBe(want);
  });
});

describe('hasRuleTargets (#1501)', () => {
  test.each([
    { name: '何もない', form: emptyConditions(), want: false },
    {
      name: 'キーワード',
      form: {
        ...emptyConditions(),
        keywords: [
          { text: 'ニュース', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
        ],
      },
      want: true,
    },
    { name: 'チャンネル種別', form: { ...emptyConditions(), channelTypes: ['BS'] }, want: true },
    { name: 'サービス', form: { ...emptyConditions(), services: ['U2VydmljZTox'] }, want: true },
    {
      name: 'ジャンル',
      form: { ...emptyConditions(), genres: [{ lv1: 7, lv2: null }] },
      want: true,
    },
    { name: '含める囲み文字', form: { ...emptyConditions(), marks: ['新'] }, want: true },
    {
      name: '平日 19 時台の時間帯だけ',
      form: {
        ...emptyConditions(),
        timeRanges: [{ weekdays: [1, 2, 3, 4, 5], startMinute: 19 * 60, durationMinutes: 60 }],
      },
      want: false,
    },
    {
      name: '最短の長さだけ',
      form: { ...emptyConditions(), minDurationSeconds: 1800 },
      want: false,
    },
    { name: '無料放送だけ', form: { ...emptyConditions(), freeOnly: true }, want: false },
    {
      name: '除外する囲み文字だけ',
      form: { ...emptyConditions(), excludeMarks: ['再'] },
      want: false,
    },
    {
      name: '除外するサービスだけ',
      form: { ...emptyConditions(), excludeServices: ['U2VydmljZTox'] },
      want: false,
    },
    {
      name: '除外するキーワードだけ',
      form: {
        ...emptyConditions(),
        excludeKeywords: [
          { text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME' as const] },
        ],
      },
      want: false,
    },
  ])('$name', ({ form, want }) => {
    expect(hasRuleTargets(form)).toBe(want);
  });
});

describe('fromConditions と toConditionsInput', () => {
  const data = (min: number | null, max: number | null): ConditionsData => ({
    keywords: [],
    keywordMatch: 'ALL',
    excludeKeywords: [],
    channelTypes: [],
    services: [],
    excludeServices: [],
    genres: [],
    excludeGenres: [],
    timeRanges: [],
    minDurationSeconds: min,
    maxDurationSeconds: max,
    freeOnly: false,
    marks: [],
    excludeMarks: [],
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

  test('番組名から読んだ作品名があれば、それをキーワードにする', () => {
    expect(
      conditionsFromProgram(
        'ガラスの仮面　＃５「ふたりの美登利」🈞',
        'U2VydmljZToxMjM',
        'ガラスの仮面',
      ).keywords,
    ).toEqual([{ text: 'ガラスの仮面', regexp: false, caseSensitive: false, fields: ['NAME'] }]);
  });

  test('作品名が空なら、番組名をキーワードにする', () => {
    expect(conditionsFromProgram('🈑ニュース7🈞', 'U2VydmljZToxMjM', '').keywords).toEqual([
      { text: 'ニュース7', regexp: false, caseSensitive: false, fields: ['NAME'] },
    ]);
  });

  test('囲み文字だけの番組名では、キーワードを入れない', () => {
    expect(conditionsFromProgram('🈑', 'U2VydmljZToxMjM').keywords).toEqual([]);
  });

  // 宣伝の番組から作るルールは、本編を録るためのものなので、宣伝の語を除外する (#2041)。
  test.each([
    {
      name: '全角の ＰＲ は、PR を大文字と小文字を区別して除外する',
      program: '連続テレビ小説「灯台の地図」ＰＲ',
      keyword: '灯台の地図',
      want: [{ text: 'PR', regexp: false, caseSensitive: true, fields: ['NAME'] }],
    },
    {
      name: 'ダイジェストを除外する',
      program: '大河ドラマ「灯台の地図」２分ダイジェスト',
      keyword: '灯台の地図',
      want: [{ text: 'ダイジェスト', regexp: false, caseSensitive: false, fields: ['NAME'] }],
    },
    {
      name: '両方あれば、両方を除外する',
      program: '灯台の地図 PR ダイジェスト',
      keyword: '灯台の地図',
      want: [
        { text: 'PR', regexp: false, caseSensitive: true, fields: ['NAME'] },
        { text: 'ダイジェスト', regexp: false, caseSensitive: false, fields: ['NAME'] },
      ],
    },
    {
      name: '英字の語の一部の PR は、宣伝とみなさない',
      program: 'SPRING PRIDE #3',
      keyword: 'SPRING PRIDE',
      want: [],
    },
    {
      name: '作品名が宣伝の語を含めば、除外しない',
      program: '名場面ダイジェスト #4',
      keyword: '名場面ダイジェスト',
      want: [],
    },
    {
      name: '宣伝の語がなければ、除外しない',
      program: '灯台の地図 #5',
      keyword: '灯台の地図',
      want: [],
    },
  ])('$name', ({ program, keyword, want }) => {
    expect(conditionsFromProgram(program, 'U2VydmljZToxMjM', keyword).excludeKeywords).toEqual(
      want,
    );
  });
});
