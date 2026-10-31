import { act, renderHook } from '@testing-library/react';
import { replaceDRCS } from 'aribb24.js';
import { afterEach, describe, expect, test } from 'vitest';

import {
  diffDRCS,
  drcsPixels,
  drcsReplacements,
  recordDRCS,
  resetDRCSForTest,
  setDRCSReplacements,
  useUnknownDRCS,
  validDRCSText,
} from './drcs';

// glyph は、外字の部品。画像は、2 画素 × 2 画素で、1 画素あたり 2 ビット。
const glyph = (bytes: number[]) => ({
  tag: 'DRCS',
  width: 2,
  height: 2,
  depth: 2,
  binary: new Uint8Array(bytes).buffer,
  combining: '',
});

afterEach(() => {
  resetDRCSForTest();
});

describe('drcsPixels', () => {
  test('reads each pixel from the most significant bit', () => {
    // 0b11_10_01_00: 左上から 3, 2, 1, 0。
    expect(drcsPixels(glyph([0b11100100]))).toEqual([1, 2 / 3, 1 / 3, 0]);
  });
});

describe('recordDRCS', () => {
  test('lists glyphs missing from the table, newest first', () => {
    const { result } = renderHook(() => useUnknownDRCS());
    expect(result.current).toEqual([]);

    act(() => recordDRCS([{ tag: 'Character' }, glyph([0x01]), glyph([0x02])]));
    // 0x02 と 0x01 の 1 バイトの MD5 (md5sum で求めた値)。
    expect(result.current.map((item) => item.md5)).toEqual([
      '9e688c58a5487b8eaf69c9e1005ad0bf',
      '55a54008ad1ba589aa210d2629c1df41',
    ]);

    // 表に登録すると、一覧から消える。大文字の MD5 でも小文字にそろえて引く。
    act(() => setDRCSReplacements([{ md5: '55A54008AD1BA589AA210D2629C1DF41', text: '♪' }]));
    expect(drcsReplacements.get('55a54008ad1ba589aa210d2629c1df41')).toBe('♪');
    expect(result.current.map((item) => item.md5)).toEqual(['9e688c58a5487b8eaf69c9e1005ad0bf']);
  });
});

describe('drcsReplacements', () => {
  // aribb24.js が置き換えに引く鍵と、ここで求める MD5 が同じであることを、aribb24.js の置き換えで確かめる。
  test('replaces glyphs in aribb24.js by the MD5 we compute', () => {
    const binary = new Uint8Array(Array.from({ length: 72 }, (_, i) => (i * 37) & 0xff)).buffer;
    const token = { ...glyph([]), width: 16, height: 18, binary };
    recordDRCS([token]);
    const { result } = renderHook(() => useUnknownDRCS());
    const key = result.current[0]!.md5;
    act(() => setDRCSReplacements([{ md5: key, text: '♪' }]));

    expect(replaceDRCS([token as never], drcsReplacements)).toEqual([
      { tag: 'Character', character: '♪', non_spacing: false },
    ]);
  });
});

describe('validDRCSText', () => {
  test.each([
    ['', false],
    ['♪', true],
    ['[字]', true],
    ['12345', false],
    // サロゲートペアは、サーバーと同じく 1 文字と数える。
    ['😀😀😀😀', true],
  ])('%j', (text, want) => {
    expect(validDRCSText(text)).toBe(want);
  });
});

describe('diffDRCS', () => {
  test('returns only changed glyphs, with null for removed ones', () => {
    const base = [
      { md5: 'a', text: '1' },
      { md5: 'b', text: '2' },
      { md5: 'c', text: '3' },
    ];
    const rows = [
      { md5: 'a', text: '1' },
      { md5: 'b', text: '22' },
      { md5: 'd', text: '4' },
    ];
    expect(diffDRCS(base, rows)).toEqual([
      { md5: 'b', text: '22' },
      { md5: 'd', text: '4' },
      { md5: 'c', text: null },
    ]);
    expect(diffDRCS(base, base)).toEqual([]);
  });
});
