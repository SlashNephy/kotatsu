import { describe, expect, test } from 'vitest';

import i18next from '../i18n';
import { followingCounts, followingDetail } from './following';

const t = i18next.t;

describe('followingCounts', () => {
  test('状態の順に、1 人以上いる状態だけを数える', () => {
    expect(
      followingCounts([
        { status: 'WANNA_WATCH' },
        { status: 'WATCHED' },
        { status: 'WANNA_WATCH' },
      ]),
    ).toEqual([
      { status: 'WATCHED', count: 1 },
      { status: 'WANNA_WATCH', count: 2 },
    ]);
  });
});

describe('followingDetail', () => {
  test.each([
    {
      name: '途中の進捗とスコアを並べる',
      viewing: { status: 'WATCHING', progress: 3, episodesCount: 12, score: 7.5 },
      want: '3話まで見た ・ 7.5 / 10',
    },
    {
      name: '見終えた作品の進捗は出さない',
      viewing: { status: 'WATCHED', progress: 12, episodesCount: 24, score: null },
      want: '',
    },
    {
      name: '全話まで進んでいれば進捗は出さない',
      viewing: { status: 'WATCHING', progress: 12, episodesCount: 12, score: null },
      want: '',
    },
    {
      name: '全話数が分からなくても進捗を出す',
      viewing: { status: 'ON_HOLD', progress: 5, episodesCount: null, score: null },
      want: '5話まで見た',
    },
  ])('$name', ({ viewing, want }) => {
    expect(followingDetail(t, viewing)).toBe(want);
  });
});
