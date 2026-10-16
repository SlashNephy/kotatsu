import { describe, expect, test } from 'vitest';

import { appendComment, commentsUntil, maxListed, type ListedComment } from './commentList';
import type { JikkyoComment } from './comments';

describe('appendComment', () => {
  test('届いた順に末尾へ足し、行を見分ける番号を振る', () => {
    let list: readonly ListedComment[] = [];
    list = appendComment(list, { at: 1000, content: 'わこつ', mail: '184' });
    list = appendComment(list, { at: 2000, content: 'きたー', mail: '' });

    expect(list).toEqual([
      { key: 0, at: 1000, content: 'わこつ' },
      { key: 1, at: 2000, content: 'きたー' },
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

describe('commentsUntil', () => {
  const comments: JikkyoComment[] = [
    { time: 1, content: 'a', mail: '' },
    { time: 2, content: '/cmd', mail: '' },
    { time: 3, content: 'b', mail: '' },
    { time: 5, content: 'c', mail: '' },
  ];

  test.each([
    { name: '再生位置までに流れたものだけを返す', position: 3, want: ['a', 'b'] },
    { name: 'ちょうどの時刻のものも含める', position: 5, want: ['a', 'b', 'c'] },
    { name: '始まる前なら空', position: 0.5, want: [] },
  ])('$name', ({ position, want }) => {
    expect(commentsUntil(comments, position).map((comment) => comment.content)).toEqual(want);
  });

  test('多すぎるときは、新しいほうから上限まで返す', () => {
    const many = Array.from({ length: maxListed + 10 }, (_, i) => ({
      time: i,
      content: `c${i}`,
      mail: '',
    }));

    const shown = commentsUntil(many, maxListed + 10);

    expect(shown).toHaveLength(maxListed);
    expect(shown[0]!.content).toBe('c10');
  });
});
