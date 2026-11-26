import { describe, expect, test } from 'vitest';

import { parseChoices, parseDate, parseKeywordRows, parseStrings } from './urlSearch';

describe('parseKeywordRows', () => {
  test.each([
    { name: '配列でなければ空', value: 'ニュース', want: [] },
    {
      name: '正規表現と大文字と小文字の区別を読む',
      value: [{ text: 'ニュース', regexp: true, caseSensitive: true, fields: ['NAME'] }],
      want: [{ text: 'ニュース', regexp: true, caseSensitive: true, fields: ['NAME'] }],
    },
    {
      name: '真偽値でない印は、付けない',
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
