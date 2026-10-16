import { describe, expect, test } from 'vitest';

import { broadcastRange } from './filterRange';

describe('broadcastRange', () => {
  test.each([
    { name: '区切らない', from: '', to: '', want: {} },
    {
      name: '両端を含む',
      from: '2026-10-01',
      to: '2026-10-31',
      want: {
        since: new Date(2026, 9, 1).toISOString(),
        until: new Date(2026, 10, 1).toISOString(),
      },
    },
    {
      name: '始まりだけ',
      from: '2026-10-16',
      to: '',
      want: { since: new Date(2026, 9, 16).toISOString() },
    },
    { name: '日付でない値は区切らない', from: 'yesterday', to: '', want: {} },
  ])('$name', ({ from, to, want }) => {
    expect(broadcastRange(from, to)).toEqual(want);
  });
});
