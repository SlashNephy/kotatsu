import { describe, expect, it } from 'vitest';

import { parseUnicodeRange, usesExtendedFont } from './broadcastFont';

describe('parseUnicodeRange', () => {
  it.each([
    {
      name: '1 字と範囲を並べた書き方',
      value: 'U+4E02,U+4E04-4E05,U+20B9F',
      want: [
        [0x4e02, 0x4e02],
        [0x4e04, 0x4e05],
        [0x20b9f, 0x20b9f],
      ],
    },
    { name: '小文字は書き方に合わない', value: 'U+4e02', want: null },
    { name: '空白を挟むと書き方に合わない', value: 'U+4E02, U+4E04', want: null },
    { name: '空の文字列', value: '', want: null },
    { name: '文字列でない', value: undefined, want: null },
  ])('$name', ({ value, want }) => {
    expect(parseUnicodeRange(value)).toEqual(want);
  });
});

describe('usesExtendedFont', () => {
  const ranges: [number, number][] = [
    [0x4e02, 0x4e02],
    [0x4e04, 0x4e05],
    [0x20b9f, 0x20b9f],
  ];

  it.each([
    { name: 'JIS X 0208 の字だけ', text: '今日の天気は晴れ', want: false },
    { name: '範囲の端の字', text: 'あ\u{4E05}', want: true },
    { name: '範囲の間の字は含まない', text: '\u{4E03}', want: false },
    { name: 'BMP の外の字', text: '\u{20B9F}田', want: true },
    { name: '空の文', text: '', want: false },
  ])('$name', ({ text, want }) => {
    expect(usesExtendedFont(text, ranges)).toBe(want);
  });

  it('範囲の一覧がなければ、後のファイルを使わない', () => {
    expect(usesExtendedFont('\u{4E02}', null)).toBe(false);
  });
});
