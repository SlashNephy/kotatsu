import { describe, expect, test } from 'vitest';

import { framePosition, manifestUrl, previewFrame, slideshowFrames } from './previewFrame';

describe('previewFrame', () => {
  // 10 列で 25 コマなら、3 行になる。
  const manifest = {
    width: 160,
    height: 90,
    columns: 10,
    times: Array.from({ length: 25 }, (_, i) => i * 10),
  };

  test.each([
    {
      name: '最初のコマは、左上',
      index: 0,
      want: { backgroundSize: '1000% 300%', backgroundPosition: '0% 0%' },
    },
    {
      name: '2 行目の 3 列目',
      index: 12,
      want: { backgroundSize: '1000% 300%', backgroundPosition: `${(2 / 9) * 100}% 50%` },
    },
    {
      name: '最後のコマは、3 行目の 5 列目',
      index: 24,
      want: { backgroundSize: '1000% 300%', backgroundPosition: `${(4 / 9) * 100}% 100%` },
    },
  ])('$name', ({ index, want }) => {
    expect(previewFrame(manifest, index)).toEqual(want);
  });

  test('1 行に収まるときは、コマの数を列の数にする', () => {
    expect(previewFrame({ ...manifest, times: [0, 10, 20] }, 1)).toEqual({
      backgroundSize: '300% 100%',
      backgroundPosition: '50% 0%',
    });
  });

  test.each([
    { name: '負の番号', index: -1 },
    { name: 'コマの数を超える番号', index: 25 },
  ])('$name は null', ({ index }) => {
    expect(previewFrame(manifest, index)).toBeNull();
  });
});

describe('slideshowFrames', () => {
  test.each([
    { name: '多いときは、等間隔に limit 個を選ぶ', count: 100, limit: 4, want: [0, 25, 50, 75] },
    { name: 'limit 個以下なら、すべて', count: 3, limit: 16, want: [0, 1, 2] },
    { name: 'コマがなければ、空', count: 0, limit: 16, want: [] },
  ])('$name', ({ count, limit, want }) => {
    expect(slideshowFrames(count, limit)).toEqual(want);
  });
});

describe('manifestUrl', () => {
  test.each([
    {
      name: 'preview.jpg を preview.json にする',
      url: '/media/recordings/abc/preview.jpg',
      want: '/media/recordings/abc/preview.json',
    },
    {
      name: '版 (?v=) を引き継ぐ',
      url: '/media/recordings/abc/preview.jpg?v=1791630600',
      want: '/media/recordings/abc/preview.json?v=1791630600',
    },
  ])('$name', ({ url, want }) => {
    expect(manifestUrl(url)).toBe(want);
  });
});

describe('framePosition', () => {
  // tenban が求めたコマの位置 (ピクセル) は、preview.json から切り出したコマと同じ CSS の値になる。
  test.each([
    {
      name: '行をまたいだコマ',
      count: 25,
      index: 24,
      x: 640,
      y: 180,
      sheetWidth: 1600,
      sheetHeight: 270,
    },
    {
      name: '1 行に満たないコマ',
      count: 4,
      index: 2,
      x: 320,
      y: 0,
      sheetWidth: 640,
      sheetHeight: 90,
    },
    { name: 'コマが 1 つ', count: 1, index: 0, x: 0, y: 0, sheetWidth: 160, sheetHeight: 90 },
  ])('$name', ({ count, index, x, y, sheetWidth, sheetHeight }) => {
    const manifest = {
      width: 160,
      height: 90,
      columns: 10,
      times: Array.from({ length: count }, (_, i) => i * 10),
    };
    expect(framePosition({ x, y, width: 160, height: 90, sheetWidth, sheetHeight })).toEqual(
      previewFrame(manifest, index),
    );
  });
});
