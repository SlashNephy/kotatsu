import { describe, expect, test } from 'vitest';

import { loadInterpolation } from './frameInterpolation';

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
