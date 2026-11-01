import { describe, expect, test } from 'vitest';

import { defaultMouseActions, parseMouseActions, wheelCommands } from './mouseActions';

describe('parseMouseActions', () => {
  test.each([
    { name: 'なければ既定', raw: null, want: defaultMouseActions },
    { name: '壊れていれば既定', raw: '{', want: defaultMouseActions },
    {
      name: '覚えた値を読む',
      raw: '{"wheel":"seek","middleClick":"none"}',
      want: { wheel: 'seek', middleClick: 'none' },
    },
    {
      name: '知らない値は既定にする',
      raw: '{"wheel":"zoom","middleClick":"toggleFullscreen"}',
      want: { wheel: 'volume', middleClick: 'toggleFullscreen' },
    },
  ])('$name', ({ raw, want }) => {
    expect(parseMouseActions(raw)).toEqual(want);
  });
});

describe('wheelCommands', () => {
  test.each([
    {
      name: '音量では、上へ回すと上げる',
      action: 'volume',
      accumulated: -120,
      want: { command: 'volumeUp', rest: -20 },
    },
    {
      name: '音量では、下へ回すと下げる',
      action: 'volume',
      accumulated: 120,
      want: { command: 'volumeDown', rest: 20 },
    },
    {
      name: 'シークでは、上へ回すと戻る',
      action: 'seek',
      accumulated: -100,
      want: { command: 'seekBackward', rest: 0 },
    },
    {
      name: '少ししか回していなければ、溜める',
      action: 'volume',
      accumulated: 40,
      want: { command: null, rest: 40 },
    },
    {
      name: '使わない設定なら、何もしない',
      action: 'none',
      accumulated: 500,
      want: { command: null, rest: 0 },
    },
  ] as const)('$name', ({ action, accumulated, want }) => {
    expect(wheelCommands(action, accumulated)).toEqual(want);
  });
});
