import { describe, expect, test } from 'vitest';

import { isEmpty, toConditions, type SearchForm } from './conditions';

const empty: SearchForm = {
  keyword: '',
  scope: 'name',
  excludeKeyword: '',
  channelTypes: [],
  genres: [],
  freeOnly: false,
};

describe('toConditions', () => {
  test.each([
    { name: '入力がなければ、何も絞り込まない', form: empty, want: {} },
    {
      name: '空白で区切った語は、それぞれ番組名のキーワードにする',
      form: { ...empty, keyword: ' 名探偵　コナン ' },
      want: {
        keywords: [
          { text: '名探偵', regexp: false, caseSensitive: false, fields: ['NAME'] },
          { text: 'コナン', regexp: false, caseSensitive: false, fields: ['NAME'] },
        ],
      },
    },
    {
      name: '範囲を広げると、概要と詳細も照合する',
      form: { ...empty, keyword: '野球', scope: 'all' as const },
      want: {
        keywords: [
          {
            text: '野球',
            regexp: false,
            caseSensitive: false,
            fields: ['NAME', 'DESCRIPTION', 'EXTENDED'],
          },
        ],
      },
    },
    {
      name: '除外するキーワードは、範囲によらず、すべての項目で照合する',
      form: { ...empty, excludeKeyword: '再放送' },
      want: {
        excludeKeywords: [
          {
            text: '再放送',
            regexp: false,
            caseSensitive: false,
            fields: ['NAME', 'DESCRIPTION', 'EXTENDED'],
          },
        ],
      },
    },
    {
      name: 'チャンネル種別、ジャンル、無料の番組だけ',
      form: { ...empty, channelTypes: ['GR', 'BS'], genres: [7, 1], freeOnly: true },
      want: {
        channelTypes: ['GR', 'BS'],
        genres: [
          { lv1: 7, lv2: null },
          { lv1: 1, lv2: null },
        ],
        freeOnly: true,
      },
    },
  ])('$name', ({ form, want }) => {
    expect(toConditions(form)).toEqual(want);
  });
});

describe('isEmpty', () => {
  test.each([
    { name: '入力がない', form: empty, want: true },
    { name: '空白だけのキーワード', form: { ...empty, keyword: '   ' }, want: true },
    { name: 'キーワード', form: { ...empty, keyword: 'ニュース' }, want: false },
    { name: '無料の番組だけ', form: { ...empty, freeOnly: true }, want: false },
  ])('$name', ({ form, want }) => {
    expect(isEmpty(toConditions(form))).toBe(want);
  });
});
