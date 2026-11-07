import { describe, expect, test } from 'vitest';

import { loadCmAutoSkip } from './cmAutoSkip';

describe('loadCmAutoSkip', () => {
  test.each([
    { name: '覚えていなければ飛ばさない', stored: null, want: false },
    { name: '入れたことを覚えていれば飛ばす', stored: 'true', want: true },
    { name: '切ったことを覚えていれば飛ばさない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadCmAutoSkip({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ飛ばさない', () => {
    expect(
      loadCmAutoSkip({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});
