import { act, renderHook } from '@testing-library/react';
import { replaceDRCS } from 'aribb24.js';
import { afterEach, describe, expect, test } from 'vitest';

import {
  diffDRCS,
  drcsGlyphs,
  drcsPixels,
  drcsRenderReplacements,
  drcsReplacements,
  prepareDRCS,
  recordDRCS,
  resetDRCSForTest,
  setDRCSReplacements,
  useUnknownDRCS,
  validDRCSText,
} from './drcs';
import { drcsScale, foldDRCS, scale2x, type DRCSImage } from './drcsSmooth';

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

describe('default replacements', () => {
  // libaribcaption の定義に、U+82A6 (芦) として載っている外字。
  const bundled = '012665f146f9c4dfbfe65c41c8b30c19';

  test('replaces glyphs from the bundled table before anything is registered', () => {
    expect(drcsReplacements.get(bundled)).toBe('芦');
  });

  test('prefers a registered replacement and returns to the bundled one when it is removed', () => {
    act(() => setDRCSReplacements([{ md5: bundled, text: '「' }]));
    expect(drcsReplacements.get(bundled)).toBe('「');
    act(() => setDRCSReplacements([]));
    expect(drcsReplacements.get(bundled)).toBe('芦');
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

// image は、行ごとの文字列 (# が塗り、. が空き) から、濃さの画像を作る。
const image = (rows: string[]): DRCSImage => ({
  width: rows[0]!.length,
  height: rows.length,
  pixels: Uint8Array.from(rows.join(''), (c) => (c === '#' ? 255 : 0)),
});

// rows は、濃さの画像を、行ごとの文字列に戻す。
const rows = ({ width, pixels }: DRCSImage): string[] =>
  Array.from({ length: pixels.length / width }, (_, y) =>
    Array.from(pixels.subarray(y * width, (y + 1) * width), (v) => (v >= 128 ? '#' : '.')).join(''),
  );

// enlarge は、画像を縦横 n 倍に水増しする。放送が小さい字を整数倍して送るのをまねる。
const enlarge = (source: string[], n: number): string[] =>
  source.flatMap((row) => {
    const wide = row.replace(/./g, (c) => c.repeat(n));

    return Array.from({ length: n }, () => wide);
  });

// glyph18 は、18x18 の外字。斜めの線と、縦横の線と、塗りの塊を含める。
const glyph18 = Array.from({ length: 18 }, (_, y) =>
  Array.from({ length: 18 }, (_, x) =>
    x === y || x === 3 || y === 15 || (x >= 12 && y <= 4) ? '#' : '.',
  ).join(''),
);

describe('foldDRCS', () => {
  test.each([
    {
      name: '18x18 を縦横 2 倍にした 36x36 は、18x18 に畳む',
      source: enlarge(glyph18, 2),
      want: glyph18,
    },
    { name: '4 倍の 72x72 も、18x18 に畳む', source: enlarge(glyph18, 4), want: glyph18 },
    // 列は 3 本で奇数なので畳まず、行だけを畳む。
    {
      name: '奇数の幅は、列を畳まない',
      source: ['##.', '##.', '..#', '..#'],
      want: ['##.', '..#'],
    },
    { name: '水増しでない画像は、そのまま', source: glyph18, want: glyph18 },
  ])('$name', ({ source, want }) => {
    expect(rows(foldDRCS(image(source)))).toEqual(want);
  });
});

describe('scale2x', () => {
  test.each([
    {
      name: '縦の線は、同じ太さのまっすぐな線で残る',
      source: ['.##..', '.##..', '.##..', '.##..'],
      want: Array.from({ length: 8 }, () => '..####....'),
    },
    {
      name: '横の線は、同じ太さのまっすぐな線で残る',
      source: ['....', '####', '....'],
      want: ['........', '........', '########', '########', '........', '........'],
    },
    {
      // 斜めに接する 2 点の間 (階段の角) を埋める。
      name: '斜めの 1 画素の階段の間に点が埋まる',
      source: ['....', '.#..', '..#.', '....'],
      want: [
        '........',
        '........',
        '..##....',
        '..###...',
        '...###..',
        '....##..',
        '........',
        '........',
      ],
    },
    {
      // 画像の外を空きと見ると、左上の角が欠ける。縁で折り返すので欠けない。
      name: '縁に接した塗りの角が欠けない',
      source: ['##.', '##.', '...'],
      want: ['####..', '####..', '####..', '###...', '......', '......'],
    },
  ])('$name', ({ source, want }) => {
    expect(rows(scale2x(image(source)))).toEqual(want);
  });
});

describe('drcsScale', () => {
  test.each([
    // 目標の一辺より小さい s は、s × k が目標に届く最小の 2 の冪にする。
    { size: 18, target: 72, limit: 256, want: 4 },
    { size: 16, target: 72, limit: 256, want: 8 },
    { size: 36, target: 72, limit: 256, want: 2 },
    // 目標の一辺ちょうどの s は、拡大しない。
    { size: 72, target: 72, limit: 256, want: 1 },
    // 2 倍にすると上限を越える s は、目標に届かなくても、そこで止める。
    { size: 40, target: 72, limit: 100, want: 2 },
    { size: 60, target: 72, limit: 100, want: 1 },
  ])('s = $size, target = $target, limit = $limit', ({ size, target, limit, want }) => {
    const k = drcsScale(size, target, limit);
    expect(k).toBe(want);
    expect(size * k).toBeLessThanOrEqual(Math.max(size, limit));
  });
});

describe('prepareDRCS', () => {
  // unknown は、置き換え表に無い外字。36x36 で、1 画素あたり 1 ビット。
  const unknown = {
    tag: 'DRCS',
    width: 36,
    height: 36,
    depth: 1,
    binary: new Uint8Array(Array.from({ length: 162 }, (_, i) => (i * 53) & 0xff)).buffer,
    combining: '',
  };

  test('私用領域の文字は、映像に重ねる描画器の表にだけ足し、字幕の文の表には足さない', () => {
    const { result } = renderHook(() => useUnknownDRCS());
    act(() => recordDRCS([unknown]));
    const key = result.current[0]!.md5;
    prepareDRCS([36, 36], [unknown]);

    // 映像に重ねる描画器では、私用領域の文字にし、その path を登録する。
    const [drawn] = replaceDRCS([unknown as never], drcsRenderReplacements);
    expect(drawn?.tag).toBe('Character');
    const character = (drawn as { character: string }).character;
    expect(character.codePointAt(0)).toBeGreaterThanOrEqual(0xe000);
    expect(character.codePointAt(0)).toBeLessThanOrEqual(0xf8ff);
    expect(drcsGlyphs.get(character)?.path).not.toBe('');

    // 字幕の文の表は変えないので、外字のまま (aribb24.js の TextRenderer は「〓」にする) で、一覧にも残る。
    expect(replaceDRCS([unknown as never], drcsReplacements)[0]?.tag).toBe('DRCS');
    expect(drcsReplacements.has(key)).toBe(false);
    expect(result.current.map((item) => item.md5)).toEqual([key]);

    // 管理者が登録したら、登録した文字で描く。
    act(() => setDRCSReplacements([{ md5: key, text: '♪' }]));
    expect(drcsRenderReplacements.get(key)).toBe('♪');
  });

  test.each([
    // aribb24.js は、viewBox を字の大きさ (fontsize) に縮めて描く。元の外字と同じ大きさに描けるよう、
    // 字の大きさに対する外字の大きさの分だけ、viewBox を path より広げる。左上の 1 点だけを空けた外字で、path が占める広さを確かめる。
    // 一様に塗ると水増しとして畳まれるので、1 点を空ける。
    {
      name: '字の大きさと同じ外字は、字の枠いっぱいに描く',
      fontsize: [36, 36] as const,
      width: 36,
      height: 36,
      // 36 の辺は、k = 2 と超標本化の 2 で、144 にする。
      extent: [144, 144],
      viewBox: [0, 0, 144, 144],
    },
    {
      name: '中型の外字 (18x36) は、字の枠の左半分に描く',
      fontsize: [36, 36] as const,
      width: 18,
      height: 36,
      // 長い辺 36 で k = 2 に決めるので、幅は 18 × 4 = 72 にする。
      extent: [72, 144],
      viewBox: [0, 0, 144, 144],
    },
    {
      name: '字の大きさを 24 にした所の 24x24 の外字は、字の枠いっぱいに描く',
      fontsize: [36, 36] as const,
      width: 24,
      height: 24,
      composition: { tag: 'CharacterCompositionDotDesignation', horizontal: 24, vertical: 24 },
      // 24 の辺は、k = 4 と超標本化の 2 で、192 にする。
      extent: [192, 192],
      viewBox: [0, 0, 192, 192],
    },
  ])('$name', ({ fontsize, width, height, composition, extent, viewBox }) => {
    const bytes = new Uint8Array((width * height) / 8).fill(0xff);
    bytes[0] = 0x7f;
    const token = { ...unknown, width, height, binary: bytes.buffer };
    prepareDRCS(fontsize, composition === undefined ? [token] : [composition, token]);
    const [drawn] = replaceDRCS([token as never], drcsRenderReplacements);
    const glyph = drcsGlyphs.get((drawn as { character: string }).character)!;
    expect(glyph.viewBox).toEqual(viewBox);
    // path は H と V だけで書くので、その座標の最大が、path の右端と下端になる。
    const coordinates = (command: string) =>
      [...glyph.path.matchAll(new RegExp(`${command}(\\d+)`, 'g'))].map((match) =>
        Number(match[1]),
      );
    expect([Math.max(...coordinates('H')), Math.max(...coordinates('V'))]).toEqual(extent);
  });
});
