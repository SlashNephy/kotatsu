import { describe, expect, test } from 'vitest';

import { distinctTunerNames } from './tuners';

describe('distinctTunerNames', () => {
  test.each([
    { name: 'チューナーが分からなければ空', tuners: [null, undefined], want: [] },
    {
      name: '同じチューナーで続けて録ったセグメントはまとめる',
      tuners: [
        { index: 2, name: 'T0' },
        { index: 2, name: 'T0' },
      ],
      want: ['T0'],
    },
    {
      name: '別のチューナーに移って戻ったら、移った順に並べる',
      tuners: [
        { index: 2, name: 'T0' },
        null,
        { index: -1, name: 'remote / T1' },
        { index: 2, name: 'T0' },
      ],
      want: ['T0', 'remote / T1', 'T0'],
    },
  ])('$name', ({ tuners, want }) => {
    expect(distinctTunerNames(tuners)).toEqual(want);
  });
});
