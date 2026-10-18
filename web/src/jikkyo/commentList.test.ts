import { describe, expect, test } from 'vitest';

import {
  appendComment,
  liveForce,
  countUntil,
  listedUntil,
  maxListed,
  type ListedComment,
} from './commentList';
import type { JikkyoComment } from './comments';

describe('appendComment', () => {
  test('届いた順に末尾へ足し、行を見分ける番号を振る', () => {
    let list: readonly ListedComment[] = [];
    list = appendComment(list, { at: 1000, content: 'わこつ', mail: '184' });
    list = appendComment(list, { at: 2000, content: 'きたー', mail: '', userId: 'u1' });

    // ユーザー ID は、行からミュートできるよう残す (#767)。
    expect(list).toEqual([
      { key: 0, at: 1000, content: 'わこつ' },
      { key: 1, at: 2000, content: 'きたー', userId: 'u1' },
    ]);
  });

  test.each([
    { name: '空のコメント', content: '  ' },
    { name: '運営のコマンド', content: '/nicoad {}' },
  ])('$name は足さない', ({ content }) => {
    expect(appendComment([], { at: 0, content, mail: '' })).toEqual([]);
  });

  test('上限を超えたら、古いものから捨てる', () => {
    let list: readonly ListedComment[] = [];
    for (let i = 0; i < maxListed + 3; i++) {
      list = appendComment(list, { at: i, content: `c${i}`, mail: '' });
    }

    expect(list).toHaveLength(maxListed);
    expect(list[0]).toEqual({ key: 3, at: 3, content: 'c3' });
    expect(list.at(-1)).toEqual({
      key: maxListed + 2,
      at: maxListed + 2,
      content: `c${maxListed + 2}`,
    });
  });
});

describe('countUntil と listedUntil', () => {
  const comments: JikkyoComment[] = [
    { time: 1, content: 'a', mail: '' },
    { time: 2, content: '/cmd', mail: '' },
    { time: 3, content: 'b', mail: '' },
    { time: 5, content: 'c', mail: '' },
  ];

  test.each([
    {
      name: '再生位置までに流れたものだけを返す',
      position: 3,
      want: [
        { index: 0, content: 'a' },
        { index: 2, content: 'b' },
      ],
    },
    {
      name: 'ちょうどの時刻のものも含める',
      position: 5,
      want: [
        { index: 0, content: 'a' },
        { index: 2, content: 'b' },
        { index: 3, content: 'c' },
      ],
    },
    { name: '始まる前なら空', position: 0.5, want: [] },
  ])('$name', ({ position, want }) => {
    const listed = listedUntil(comments, countUntil(comments, position));

    expect(listed.map(({ index, comment }) => ({ index, content: comment.content }))).toEqual(want);
  });

  test('多すぎるときは、新しいほうから上限まで返す', () => {
    const many = Array.from({ length: maxListed + 10 }, (_, i) => ({
      time: i,
      content: `c${i}`,
      mail: '',
    }));

    const listed = listedUntil(many, countUntil(many, maxListed + 10));

    expect(listed).toHaveLength(maxListed);
    expect(listed[0]).toEqual({ index: 10, comment: many[10] });
  });
});

describe('liveForce', () => {
  test.each([
    {
      name: 'つないで 1 分を過ぎたら、直近 1 分の数',
      arrivals: [1_000, 55_000, 70_000, 100_000],
      now: 110_000,
      want: 3,
    },
    {
      name: '1 分たたない間は、1 分あたりに直す',
      arrivals: [1_000, 5_000, 20_000],
      now: 30_000,
      want: 6,
    },
    { name: 'つないで 10 秒たたない間は出さない', arrivals: [1_000], now: 9_000, want: null },
  ])('$name', ({ arrivals, now, want }) => {
    expect(liveForce(arrivals, now, 0)).toBe(want);
  });
});
