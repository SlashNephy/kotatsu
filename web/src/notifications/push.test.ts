import { describe, expect, test } from 'vitest';

import { base64urlToBytes } from './push';

describe('base64urlToBytes', () => {
  test.each([
    { name: '詰め物のない base64url', value: 'AQID_-8', want: [1, 2, 3, 255, 239] },
    { name: '4 の倍数の長さ', value: 'AAEC', want: [0, 1, 2] },
  ])('$name', ({ value, want }) => {
    expect([...base64urlToBytes(value)]).toEqual(want);
  });
});
