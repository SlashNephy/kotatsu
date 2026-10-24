import { describe, expect, test } from 'vitest';

import { decodeGenre, encodeGenre } from './guideView';

describe('ジャンルの条件と選択肢の値', () => {
  test.each([
    { name: '大分類のすべて', genre: { lv1: 7, lv2: null }, value: '7' },
    { name: '中分類', genre: { lv1: 7, lv2: 0 }, value: '7:0' },
    { name: '大分類の 0 (ニュース)', genre: { lv1: 0, lv2: null }, value: '0' },
  ])('$name', ({ genre, value }) => {
    expect(encodeGenre(genre)).toBe(value);
    expect(decodeGenre(value)).toEqual(genre);
  });

  test('lv2 が undefined でも、大分類のすべてとして扱う', () => {
    expect(encodeGenre({ lv1: 3, lv2: undefined })).toBe('3');
  });
});
