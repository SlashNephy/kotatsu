import { describe, expect, it } from 'vitest';

import { highlightRanges, type HighlightKeyword } from './highlight';

const literal = (text: string, caseSensitive = false): HighlightKeyword => ({
  text,
  regexp: false,
  caseSensitive,
});
const regexp = (text: string, caseSensitive = false): HighlightKeyword => ({
  text,
  regexp: true,
  caseSensitive,
});

// marked は、text のマークを付けた所を [ ] で囲んで返す。
function marked(text: string, keywords: HighlightKeyword[]): string {
  let result = '';
  let at = 0;
  for (const range of highlightRanges(text, keywords)) {
    result += `${text.slice(at, range.start)}[${text.slice(range.start, range.end)}]`;
    at = range.end;
  }

  return result + text.slice(at);
}

describe('highlightRanges', () => {
  it.each([
    {
      name: 'a literal keyword',
      text: 'こたつの時間',
      keywords: [literal('こたつ')],
      want: '[こたつ]の時間',
    },
    {
      name: 'every occurrence',
      text: 'ねこ と ねこ',
      keywords: [literal('ねこ')],
      want: '[ねこ] と [ねこ]',
    },
    // 全角の英数字は、正規化して照合し、元の文字にマークを付ける。
    {
      name: 'full-width letters',
      text: 'ＮＨＫニュース',
      keywords: [literal('nhk')],
      want: '[ＮＨＫ]ニュース',
    },
    {
      name: 'case sensitive',
      text: 'News news',
      keywords: [literal('news', true)],
      want: 'News [news]',
    },
    // 波ダッシュと全角チルダは、どちらで書いても当たる (#2041)。
    {
      name: 'wave dashes',
      text: '作品名〜副題〜',
      keywords: [literal('作品名～副題～')],
      want: '[作品名〜副題〜]',
    },
    {
      name: 'dash variants',
      text: '作品名―副題―',
      keywords: [literal('名-副')],
      want: '作品[名―副]題―',
    },
    {
      name: 'a regular expression',
      text: '灯台の地図 #12',
      keywords: [regexp('#[0-9]+$')],
      want: '灯台の地図 [#12]',
    },
    {
      name: 'an invalid regular expression',
      text: '灯台',
      keywords: [regexp('(?P<x>灯台)')],
      want: '灯台',
    },
    {
      name: 'overlapping keywords',
      text: 'こたつ特集',
      keywords: [literal('こたつ'), literal('つ特')],
      want: '[こたつ特]集',
    },
    { name: 'no keyword', text: 'こたつ', keywords: [literal(' ')], want: 'こたつ' },
  ])('$name', ({ text, keywords, want }) => {
    expect(marked(text, keywords)).toBe(want);
  });
});
