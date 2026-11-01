import { describe, expect, test } from 'vitest';

import { loadAudioDelay, loadResetAudioDelay, stepAudioDelay } from './audioDelay';

// storage は、getItem だけを持つ Storage の代わり。
const storage = (values: Record<string, string | undefined>) => ({
  getItem: (key: string) => values[key] ?? null,
});

describe('loadAudioDelay', () => {
  test.each([
    { name: '覚えた値を読む', values: { 'kotatsu.player.audioDelayMs': '120' }, want: 120 },
    { name: 'なければ 0', values: {}, want: 0 },
    {
      name: '上限を超えていれば上限にする',
      values: { 'kotatsu.player.audioDelayMs': '9000' },
      want: 500,
    },
    { name: '数でなければ 0', values: { 'kotatsu.player.audioDelayMs': 'x' }, want: 0 },
  ])('$name', ({ values, want }) => {
    expect(loadAudioDelay(storage(values))).toBe(want);
  });
});

describe('loadResetAudioDelay', () => {
  test.each([
    { values: { 'kotatsu.player.resetAudioDelayOnChannelChange': 'true' }, want: true },
    { values: {}, want: false },
  ])('$values', ({ values, want }) => {
    expect(loadResetAudioDelay(storage(values))).toBe(want);
  });
});

describe('stepAudioDelay', () => {
  test.each([
    { name: '20 ms ずつ増やす', ms: 100, direction: 1, want: 120 },
    { name: '20 ms ずつ減らす', ms: 100, direction: -1, want: 80 },
    { name: '0 より下げない', ms: 0, direction: -1, want: 0 },
    { name: '上限で止める', ms: 500, direction: 1, want: 500 },
    { name: '刻みからずれた値は、刻みにそろえる', ms: 105, direction: 1, want: 120 },
  ] as const)('$name', ({ ms, direction, want }) => {
    expect(stepAudioDelay(ms, direction)).toBe(want);
  });
});
