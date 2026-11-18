import { describe, expect, test } from 'vitest';

import { loadRecordOnDevice } from './recordOnDevice';

describe('loadRecordOnDevice', () => {
  test.each([
    { name: '覚えていなければ記録する', stored: null, want: true },
    { name: '切ったことを覚えていれば記録しない', stored: 'false', want: false },
    { name: '入れたことを覚えていれば記録する', stored: 'true', want: true },
  ])('$name', ({ stored, want }) => {
    expect(loadRecordOnDevice({ getItem: () => stored })).toBe(want);
  });

  test('見たドラマは、見たアニメと別に覚える (#1649)', () => {
    const stored: Record<string, string> = { 'kotatsu.animeRecords.recordOnDevice': 'false' };
    const storage = { getItem: (key: string) => stored[key] ?? null };

    expect(loadRecordOnDevice(storage, 'anime')).toBe(false);
    expect(loadRecordOnDevice(storage, 'drama')).toBe(true);
  });

  test('読めなければ記録する', () => {
    expect(
      loadRecordOnDevice({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(true);
  });
});
