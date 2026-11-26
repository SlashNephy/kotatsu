import { describe, expect, test } from 'vitest';

import { emptyConditions, emptyKeyword } from '../conditions/form';
import { archiveParam, formFromSearch, parseProgramSearch } from './searchParams';

describe('parseProgramSearch', () => {
  test.each([
    { name: '空のクエリ', search: {}, want: {} },
    {
      name: '知っている値を読む',
      search: {
        conditions: { keywords: [{ text: 'ニュース', fields: ['NAME'] }] },
        archive: 7,
        program: 'UHJvZ3JhbTox',
      },
      want: {
        conditions: { keywords: [{ ...emptyKeyword(), text: 'ニュース' }] },
        archive: 7,
        program: 'UHJvZ3JhbTox',
      },
    },
    {
      name: '条件のない conditions は載せない',
      search: { conditions: { freeOnly: false } },
      want: {},
    },
    ...[0, -1, 1.5, Number.NaN, 3651, '7'].map((archive) => ({
      name: `範囲の外か整数でない日数 (${String(archive)}) は破棄する`,
      search: { archive },
      want: {},
    })),
    { name: '空の番組の ID は破棄する', search: { program: '' }, want: {} },
  ])('$name', ({ search, want }) => {
    expect(parseProgramSearch(search)).toEqual(want);
  });
});

describe('archiveParam', () => {
  test.each([
    { days: 0, want: 1 },
    { days: 30, want: 30 },
    { days: 5000, want: 3650 },
  ])('$days 日は $want 日', ({ days, want }) => {
    expect(archiveParam(days)).toBe(want);
  });
});

describe('formFromSearch', () => {
  test('条件がなければ、キーワードの行を 1 つ開けておく', () => {
    expect(formFromSearch(undefined)).toEqual({ ...emptyConditions(), keywords: [emptyKeyword()] });
  });
});
