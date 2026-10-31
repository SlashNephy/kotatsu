import { describe, expect, test } from 'vitest';

import { cropForMode, cropMode, cropSource, cropStyle, noCrop } from './crop';

describe('cropStyle', () => {
  test('切り抜かなければ、変形を掛けない', () => {
    expect(cropStyle(noCrop)).toEqual({});
  });

  test.each([
    { origin: 'top-left' as const, want: '0% 0%' },
    { origin: 'top-right' as const, want: '100% 0%' },
    { origin: 'center' as const, want: '50% 50%' },
  ])('起点 $origin で拡大する', ({ origin, want }) => {
    expect(cropStyle({ scale: 1.25, origin })).toEqual({
      transform: 'scale(1.25)',
      transformOrigin: want,
    });
  });
});

describe('cropSource', () => {
  // 1920x1080 を 1.25 倍にすると、見えるのは 1536x864。
  test.each([
    { name: '左上を拡大すると、左上の範囲', origin: 'top-left' as const, want: { x: 0, y: 0 } },
    { name: '右上を拡大すると、右上の範囲', origin: 'top-right' as const, want: { x: 384, y: 0 } },
    {
      name: '右下を拡大すると、右下の範囲',
      origin: 'bottom-right' as const,
      want: { x: 384, y: 216 },
    },
    { name: '中央を拡大すると、真ん中の範囲', origin: 'center' as const, want: { x: 192, y: 108 } },
  ])('$name', ({ origin, want }) => {
    expect(cropSource({ scale: 1.25, origin }, 1920, 1080)).toEqual({
      ...want,
      width: 1536,
      height: 864,
    });
  });

  test('切り抜かなければ、全体', () => {
    expect(cropSource(noCrop, 1920, 1080)).toEqual({ x: 0, y: 0, width: 1920, height: 1080 });
  });
});

describe('横に伸ばす (#824)', () => {
  test('変形は、横と縦の倍率を分けて掛ける', () => {
    expect(cropStyle(cropForMode('stretch'))).toEqual({
      transform: `scale(${4 / 3}, 1)`,
      transformOrigin: '50% 50%',
    });
  });

  test('キャプチャで切り出す範囲は、横だけ狭くなる', () => {
    expect(cropSource(cropForMode('stretch'), 1440, 1080)).toEqual({
      x: 180,
      y: 0,
      width: 1080,
      height: 1080,
    });
  });
});

describe('cropMode', () => {
  test.each([
    { name: '切り抜かない', crop: noCrop, want: 'none' },
    { name: '4:3 の左右の黒い縁を消す', crop: cropForMode('pillarbox'), want: 'pillarbox' },
    { name: '4:3 を横に伸ばす', crop: cropForMode('stretch'), want: 'stretch' },
    { name: 'L 字の切り抜き', crop: { scale: 1.25, origin: 'top-left' as const }, want: 'lShape' },
  ])('$name', ({ crop, want }) => {
    expect(cropMode(crop)).toBe(want);
  });
});
