import { describe, expect, test } from 'vitest';

import { formatVersion } from './version';

describe('formatVersion', () => {
  test.each([
    {
      name: 'コミットの先頭 7 文字を添える',
      v: { version: '1.2.3', revision: 'fedcba9876543210', modified: false },
      want: '1.2.3 (fedcba9)',
    },
    {
      name: 'コミットしていない変更があれば、dirty を付ける',
      v: { version: 'dev', revision: '0123456789abcdef', modified: true },
      want: 'dev (0123456-dirty)',
    },
    {
      name: 'コミットが分からなければ、版だけ',
      v: { version: 'dev', revision: null, modified: false },
      want: 'dev',
    },
  ])('$name', ({ v, want }) => {
    expect(formatVersion(v)).toBe(want);
  });
});
