import { describe, expect, test } from 'vitest';

import {
  formFromSearch,
  parseRecordingListSearch,
  recordingFilter,
  recordingKeyword,
  searchFromForm,
} from './listSearch';

const keyword = (text: string) => ({ ...recordingKeyword(), text });
const caption = (text: string) => ({ text, regexp: false, caseSensitive: false });

describe('parseRecordingListSearch', () => {
  test.each([
    { name: '空のクエリは絞り込まない', search: {}, want: {} },
    {
      name: '知っている値を読む',
      search: {
        rule: 'UnVsZTox',
        keywords: [{ text: ' ニュース ', regexp: false, caseSensitive: false, fields: ['NAME'] }],
        keywordMatch: 'ANY',
        tags: ['VGFnOjE='],
        states: ['FAILED', 'COMPLETED'],
        services: ['U2VydmljZTox'],
        genres: [7, 0],
        from: '2026-11-01',
        to: '2026-11-30',
        order: 'oldest',
      },
      want: {
        rule: 'UnVsZTox',
        keywords: [{ text: 'ニュース', regexp: false, caseSensitive: false, fields: ['NAME'] }],
        keywordMatch: 'ANY',
        tags: ['VGFnOjE='],
        // 並びは、絞り込みの選択肢の順にそろえる。
        states: ['COMPLETED', 'FAILED'],
        services: ['U2VydmljZTox'],
        genres: [0, 7],
        from: '2026-11-01',
        to: '2026-11-30',
        order: 'oldest',
      },
    },
    {
      name: '知らない値と形の違う値は破棄する',
      search: {
        rule: '',
        keywords: [
          { text: '', fields: ['NAME'] },
          // 録画は詳細では探さないので、詳細だけの行は破棄する。
          { text: 'ドラマ', fields: ['EXTENDED'] },
          'ニュース',
        ],
        keywordMatch: 'ALL',
        tags: 'VGFnOjE=',
        states: ['RESERVED'],
        genres: [16, '7'],
        from: '2026-02-30',
        to: 'yesterday',
        order: 'NEWEST_FIRST',
      },
      want: {},
    },
    {
      name: '含めるキーワードがなければ、照合のしかたを載せない',
      search: { keywordMatch: 'ANY', excludeKeywords: [keyword('再放送')] },
      want: { excludeKeywords: [keyword('再放送')] },
    },
    {
      name: '字幕のキーワードは、項目を持たずに読む',
      search: {
        captionKeywords: [
          { text: ' 誰なの ', regexp: false, caseSensitive: false, fields: ['NAME'] },
          { text: '\\d+度', regexp: true, caseSensitive: true },
          { text: ' ' },
          '北海道',
        ],
        captionKeywordMatch: 'ANY',
      },
      want: {
        captionKeywords: [
          { text: '誰なの', regexp: false, caseSensitive: false },
          { text: '\\d+度', regexp: true, caseSensitive: true },
        ],
        captionKeywordMatch: 'ANY',
      },
    },
    {
      name: '字幕のキーワードがなければ、その照合のしかたを載せない',
      search: { captionKeywords: [{ text: '' }], captionKeywordMatch: 'ANY' },
      want: {},
    },
  ])('$name', ({ search, want }) => {
    expect(parseRecordingListSearch(search)).toEqual(want);
  });
});

describe('recordingFilter', () => {
  test.each([
    { name: '絞り込まない', search: {}, want: {} },
    {
      name: 'キーワードには照合のしかたと除外を添える',
      search: { keywords: [keyword('ニュース')] },
      want: { keywords: [keyword('ニュース')], keywordMatch: 'ALL', excludeKeywords: [] },
    },
    {
      name: '字幕の照合のしかたは、いずれかに一致するときだけ送る',
      search: { captionKeywords: [caption('誰なの')] },
      want: { captionKeywords: [caption('誰なの')] },
    },
    {
      name: '字幕のいずれかに一致',
      search: { captionKeywords: [caption('誰なの')], captionKeywordMatch: 'ANY' as const },
      want: { captionKeywords: [caption('誰なの')], captionKeywordMatch: 'ANY' },
    },
    {
      name: 'クエリの名前を、tenban の項目の名前にする',
      search: {
        rule: 'UnVsZTox',
        tags: ['VGFnOjE='],
        states: ['FAILED' as const],
        services: ['U2VydmljZTox'],
        genres: [7],
        from: '2026-11-01',
        to: '2026-11-30',
      },
      want: {
        ruleId: 'UnVsZTox',
        tagIds: ['VGFnOjE='],
        states: ['FAILED'],
        serviceIds: ['U2VydmljZTox'],
        genres: [7],
        since: new Date(2026, 10, 1).toISOString(),
        until: new Date(2026, 11, 1).toISOString(),
      },
    },
  ])('$name', ({ search, want }) => {
    expect(recordingFilter(search)).toEqual(want);
  });
});

describe('searchFromForm', () => {
  test('空のフォームは、ルールと並び順だけを残す', () => {
    const current = { rule: 'UnVsZTox', order: 'oldest' as const, tags: ['VGFnOjE='] };

    expect(searchFromForm(formFromSearch({}), current)).toEqual({
      rule: 'UnVsZTox',
      order: 'oldest',
    });
  });

  test('クエリから作ったフォームは、同じクエリに戻る', () => {
    const search = parseRecordingListSearch({
      keywords: [keyword('ニュース')],
      keywordMatch: 'ANY',
      excludeKeywords: [keyword('再放送')],
      captionKeywords: [caption('誰なの'), { text: '寺田', regexp: true, caseSensitive: true }],
      captionKeywordMatch: 'ANY',
      tags: ['VGFnOjE='],
      states: ['FAILED'],
      services: ['U2VydmljZTox'],
      genres: [7],
      from: '2026-11-01',
      to: '2026-11-30',
    });

    expect(searchFromForm(formFromSearch(search), search)).toEqual(search);
  });
});
