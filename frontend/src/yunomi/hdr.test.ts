import { describe, expect, test } from 'vitest';

import { loadHDROutput } from './hdr';

describe('loadHDROutput', () => {
  test.each([
    { name: '覚えていなければ、擬似 HDR にしない', stored: null, want: false },
    { name: '入れたと覚えていれば、擬似 HDR にする', stored: 'true', want: true },
    { name: '切ったと覚えていれば、擬似 HDR にしない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadHDROutput({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ、擬似 HDR にしない', () => {
    expect(
      loadHDROutput({
        getItem: () => {
          throw new Error('SecurityError');
        },
      }),
    ).toBe(false);
  });
});
