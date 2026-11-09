import { describe, expect, it } from 'vitest';

import { rankByJikkyoForce } from './jikkyoRanking';

function row(name: string, jikkyo: { id: string; force: number | null } | null) {
  return { name, service: { jikkyo } };
}

describe('rankByJikkyoForce', () => {
  it.each([
    {
      title: '勢いの強い順に並べる',
      rows: [row('a', { id: 'jk1', force: 3 }), row('b', { id: 'jk2', force: 19 })],
      want: ['b', 'a'],
    },
    {
      title: '勢いが 0 の局と、データのない局を除く',
      rows: [
        row('zero', { id: 'jk1', force: 0 }),
        row('null', { id: 'jk2', force: null }),
        row('none', null),
        row('hot', { id: 'jk4', force: 1 }),
      ],
      want: ['hot'],
    },
    {
      title: '同じ勢いなら、渡した順を保つ',
      rows: [row('a', { id: 'jk1', force: 5 }), row('b', { id: 'jk2', force: 5 })],
      want: ['a', 'b'],
    },
    {
      title: '同じ実況のチャンネルの局は、最初の 1 局だけ残す',
      rows: [
        row('main', { id: 'jk9', force: 7 }),
        row('sub', { id: 'jk9', force: 7 }),
        row('other', { id: 'jk1', force: 2 }),
      ],
      want: ['main', 'other'],
    },
  ])('$title', ({ rows, want }) => {
    expect(rankByJikkyoForce(rows).map((r) => r.name)).toEqual(want);
  });
});
