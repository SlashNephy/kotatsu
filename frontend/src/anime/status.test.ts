import { describe, expect, test } from 'vitest';

import { parseLibraryStatus } from './status';

describe('parseLibraryStatus', () => {
  test.each([
    ['WATCHING', 'WATCHING'],
    ['WANNA_WATCH', 'WANNA_WATCH'],
    ['all', 'all'],
    [undefined, 'WATCHING'],
    ['unknown', 'WATCHING'],
  ])('%s は %s にする', (value, want) => {
    expect(parseLibraryStatus(value)).toBe(want);
  });
});
