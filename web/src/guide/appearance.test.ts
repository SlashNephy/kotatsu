import { describe, expect, test } from 'vitest';

import {
  defaultGenreColors,
  defaultGuideAppearance,
  genreBackground,
  parseGuideAppearance,
} from './appearance';

describe('parseGuideAppearance', () => {
  test.each([
    { name: '覚えていなければ既定', raw: null, want: defaultGuideAppearance },
    { name: '壊れた JSON は既定', raw: '{', want: defaultGuideAppearance },
    {
      name: '覚えた密度と灰色と色を読む',
      raw: JSON.stringify({ density: 'wide', grayShopping: true, genreColors: { 7: 'pink' } }),
      want: {
        density: 'wide',
        grayShopping: true,
        genreColors: { ...defaultGenreColors, 7: 'pink' },
      },
    },
    {
      name: '知らない密度と色と、色を選べないジャンルは既定に戻す',
      raw: JSON.stringify({
        density: 'huge',
        grayShopping: 'yes',
        genreColors: { 0: '#ff0000', 15: 'red' },
      }),
      want: defaultGuideAppearance,
    },
  ])('$name', ({ raw, want }) => {
    expect(parseGuideAppearance(raw)).toEqual(want);
  });
});

describe('genreBackground', () => {
  const shopping = { lv1: 2, lv2: 4 };
  test.each([
    {
      name: '大分類の色',
      appearance: defaultGuideAppearance,
      genre: { lv1: 7, lv2: 0 },
      want: 'orange',
    },
    {
      name: '選んだ色',
      appearance: {
        ...defaultGuideAppearance,
        genreColors: { ...defaultGenreColors, 7: 'pink' as const },
      },
      genre: { lv1: 7, lv2: 0 },
      want: 'pink',
    },
    {
      name: 'ジャンルがなければ灰色',
      appearance: defaultGuideAppearance,
      genre: undefined,
      want: 'gray',
    },
    {
      name: 'その他は灰色',
      appearance: defaultGuideAppearance,
      genre: { lv1: 15, lv2: 15 },
      want: 'gray',
    },
    {
      name: '灰色にしなければ通販番組も情報の色',
      appearance: defaultGuideAppearance,
      genre: shopping,
      want: 'cyan',
    },
    {
      name: '灰色にすれば通販番組は灰色',
      appearance: { ...defaultGuideAppearance, grayShopping: true },
      genre: shopping,
      want: 'gray',
    },
    {
      name: '灰色にしても通販でない情報番組は情報の色',
      appearance: { ...defaultGuideAppearance, grayShopping: true },
      genre: { lv1: 2, lv2: 5 },
      want: 'cyan',
    },
  ])('$name', ({ appearance, genre, want }) => {
    expect(genreBackground(appearance, genre)).toBe(`var(--mantine-color-${want}-light)`);
  });
});
