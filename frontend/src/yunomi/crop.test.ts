import { describe, expect, test } from 'vitest';

import {
  cropForMode,
  cropMode,
  cropSource,
  cropStyle,
  isCropped,
  noCrop,
  screenCrop,
} from './crop';

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
    { name: '画面いっぱいに広げる', crop: cropForMode('fill'), want: 'fill' },
    // 全画面で拡大した後も、メニューでは、画面いっぱいに広げる選択肢のまま。
    {
      name: '画面いっぱいに広げて拡大した',
      crop: screenCrop(cropForMode('fill'), true, 2400, 1080),
      want: 'fill',
    },
  ])('$name', ({ crop, want }) => {
    expect(cropMode(crop)).toBe(want);
  });
});

describe('画面いっぱいに広げる (#1599)', () => {
  test.each([
    // 16:9 より横長の画面 (20:9) では、横を画面の幅に合わせ、上下を切る。
    {
      name: '横長の画面',
      fullscreen: true,
      width: 2400,
      height: 1080,
      want: 2400 / 1080 / (16 / 9),
    },
    // 16:9 より縦長の画面 (4:3) では、縦を画面の高さに合わせ、左右を切る。
    { name: '縦長の画面', fullscreen: true, width: 1024, height: 768, want: 16 / 9 / (1024 / 768) },
    { name: '16:9 の画面', fullscreen: true, width: 1920, height: 1080, want: 1 },
    // 縦向きの画面では、覆うと映像の大半が切れるので、拡大しない。
    { name: '縦向きの画面', fullscreen: true, width: 412, height: 915, want: 1 },
    // 全画面でなければ、枠と画面の比率は同じなので、拡大しない。
    { name: '全画面でない', fullscreen: false, width: 2400, height: 1080, want: 1 },
  ])('$name', ({ fullscreen, width, height, want }) => {
    const crop = screenCrop(cropForMode('fill'), fullscreen, width, height);
    expect(crop.scale).toBeCloseTo(want);
    expect(crop.origin).toBe('center');
  });

  test('ほかの表示の比率は、画面の比率で変えない', () => {
    const lShape = cropForMode('lShape');
    expect(screenCrop(lShape, true, 2400, 1080)).toBe(lShape);
  });

  test('キャプチャは、拡大していない元の切り抜きで、16:9 の枠をそのまま写す', () => {
    expect(isCropped(cropForMode('fill'))).toBe(false);
    expect(cropSource(cropForMode('fill'), 1920, 1080)).toEqual({
      x: 0,
      y: 0,
      width: 1920,
      height: 1080,
    });
  });
});
