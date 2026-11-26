import { describe, expect, test } from 'vitest';

import { emptyConditions, emptyKeyword } from './form';
import {
  conditionsFromSearch,
  conditionsSearch,
  parseChoices,
  parseDate,
  parseKeywordRows,
  parseStrings,
} from './urlSearch';

describe('parseKeywordRows', () => {
  test.each([
    { name: '配列でなければ空', value: 'ニュース', want: [] },
    {
      name: '正規表現と大文字と小文字の区別を読む',
      value: [{ text: 'ニュース', regexp: true, caseSensitive: true, fields: ['NAME'] }],
      want: [{ text: 'ニュース', regexp: true, caseSensitive: true, fields: ['NAME'] }],
    },
    {
      name: '真偽値でない正規表現と大文字と小文字の区別は、オフとして読む',
      value: [{ text: 'ニュース', regexp: 'true', caseSensitive: 1, fields: ['NAME'] }],
      want: [{ text: 'ニュース', regexp: false, caseSensitive: false, fields: ['NAME'] }],
    },
    {
      name: '項目は、選べるものだけを、選べる順に 1 つずつ読む',
      value: [{ text: 'ニュース', fields: ['DESCRIPTION', 'EXTENDED', 'NAME', 'NAME'] }],
      want: [
        { text: 'ニュース', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
      ],
    },
    {
      name: 'オブジェクトでない行と、語のない行は破棄する',
      value: [null, 'ニュース', { text: '  ', fields: ['NAME'] }, { fields: ['NAME'] }],
      want: [],
    },
  ])('$name', ({ value, want }) => {
    expect(parseKeywordRows(value, ['NAME', 'DESCRIPTION'])).toEqual(want);
  });
});

describe('parseStrings', () => {
  test.each([
    { name: '配列でなければ空', value: 'a', want: [] },
    {
      name: '空の文字列と文字列でない値を破棄し、同じ値をまとめる',
      value: ['a', '', 1, 'a'],
      want: ['a'],
    },
  ])('$name', ({ value, want }) => {
    expect(parseStrings(value)).toEqual(want);
  });
});

describe('parseChoices', () => {
  test.each([
    { name: '配列でなければ空', value: 'FAILED', want: [] },
    {
      name: '同じ値をまとめ、選択肢の順にする',
      value: ['FAILED', 'FAILED', 'COMPLETED'],
      want: ['COMPLETED', 'FAILED'],
    },
  ])('$name', ({ value, want }) => {
    expect(parseChoices(value, ['COMPLETED', 'FAILED'])).toEqual(want);
  });
});

describe('parseDate', () => {
  test.each([
    { name: 'うるう年の 2 月 29 日は読む', value: '2024-02-29', want: '2024-02-29' },
    { name: 'うるう年でない 2 月 29 日は破棄する', value: '2026-02-29', want: undefined },
    { name: '形の違う値は破棄する', value: '2026/11/01', want: undefined },
    { name: '文字列でなければ破棄する', value: 20261101, want: undefined },
  ])('$name', ({ value, want }) => {
    expect(parseDate(value)).toBe(want);
  });
});

describe('conditionsSearch', () => {
  test.each([
    { name: '条件がなければ空', form: emptyConditions(), want: {} },
    {
      name: '編集の途中の行と既定の値は載せない',
      form: {
        ...emptyConditions(),
        keywords: [{ ...emptyKeyword(), text: ' ニュース ' }, emptyKeyword()],
        timeRanges: [{ weekdays: [], startMinute: 0, durationMinutes: 60 }],
      },
      want: { keywords: [{ ...emptyKeyword(), text: 'ニュース' }] },
    },
    {
      name: '含めるキーワードがなければ、照合のしかたを載せない',
      form: { ...emptyConditions(), keywordMatch: 'ANY' as const, freeOnly: true },
      want: { freeOnly: true },
    },
    {
      name: 'クエリを読むときと同じ決まりで、値をそろえる',
      form: {
        ...emptyConditions(),
        // 選んだ順の項目は、項目の並びにそろえる。
        keywords: [
          { ...emptyKeyword(), fields: ['EXTENDED' as const, 'NAME' as const], text: 'a' },
        ],
        // 長さが 0 の時間帯は、クエリから読むときに破棄するので、載せない。
        timeRanges: [{ weekdays: [1], startMinute: 0, durationMinutes: 0 }],
        genres: [
          { lv1: 7, lv2: null },
          { lv1: 7, lv2: null },
        ],
      },
      want: {
        keywords: [{ ...emptyKeyword(), fields: ['NAME', 'EXTENDED'], text: 'a' }],
        genres: [{ lv1: 7, lv2: null }],
      },
    },
  ])('$name', ({ form, want }) => {
    expect(conditionsSearch(form)).toEqual(want);
  });

  test('クエリから読み直しても同じ', () => {
    const search = conditionsSearch({
      ...emptyConditions(),
      keywords: [{ ...emptyKeyword(), text: 'ニュース' }],
      keywordMatch: 'ANY',
      excludeKeywords: [{ ...emptyKeyword(), text: '再放送' }],
      channelTypes: ['GR'],
      services: ['U2VydmljZTox'],
      excludeServices: ['U2VydmljZToy'],
      genres: [{ lv1: 7, lv2: 0 }],
      excludeGenres: [{ lv1: 0, lv2: null }],
      timeRanges: [{ weekdays: [1, 2], startMinute: 1200, durationMinutes: 60 }],
      minDurationSeconds: 600,
      maxDurationSeconds: 3600,
      freeOnly: true,
      marks: ['新'],
      excludeMarks: ['再'],
    });

    expect(conditionsSearch(conditionsFromSearch(search))).toEqual(search);
  });
});

describe('conditionsFromSearch', () => {
  test('形の違う値と範囲の外の値は破棄する', () => {
    expect(
      conditionsFromSearch({
        keywordMatch: 'ANY',
        channelTypes: 'GR',
        genres: [{ lv1: 16 }, { lv1: 7, lv2: 99 }],
        timeRanges: [{ weekdays: [7], startMinute: 0, durationMinutes: 60 }],
        minDurationSeconds: -1,
        maxDurationSeconds: 2 ** 31,
        freeOnly: 'true',
      }),
    ).toEqual({ ...emptyConditions(), genres: [{ lv1: 7, lv2: null }] });
  });
});
