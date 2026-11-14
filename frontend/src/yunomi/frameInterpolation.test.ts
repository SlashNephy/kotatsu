import { describe, expect, test } from 'vitest';

import { loadInterpolation, loadInterpolationRate } from './frameInterpolation';

describe('loadInterpolation', () => {
  test.each([
    { name: '覚えていなければ補間しない', stored: null, want: false },
    { name: '入れたことを覚えていれば補間する', stored: 'true', want: true },
    { name: '切ったことを覚えていれば補間しない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadInterpolation({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ補間しない', () => {
    expect(
      loadInterpolation({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});

describe('loadInterpolationRate', () => {
  test.each([
    { name: '覚えていなければ、画面の書き換えに合わせる', stored: null, want: null },
    { name: '120 を覚えていれば 120', stored: '120', want: 120 },
    { name: '60 を覚えていれば 60', stored: '60', want: 60 },
    { name: '選べない値なら、画面の書き換えに合わせる', stored: '90', want: null },
    { name: '数でなければ、画面の書き換えに合わせる', stored: 'fast', want: null },
  ])('$name', ({ stored, want }) => {
    expect(loadInterpolationRate({ getItem: () => stored })).toBe(want);
  });
});
