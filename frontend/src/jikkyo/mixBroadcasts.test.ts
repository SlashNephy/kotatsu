import { describe, expect, test } from 'vitest';

import type { JikkyoComment } from './comments';
import { matchMarkers, mapComments, mixBroadcast } from './mixBroadcasts';
import type { PartMarker, PartSymbol } from './partMarkers';

const markers = (rows: [PartSymbol, number][]): PartMarker[] =>
  rows.map(([part, time]) => ({ part, time, noCmBefore: false }));

const comment = (time: number, content = 'c'): JikkyoComment => ({ time, content, mail: '' });

describe('matchMarkers', () => {
  test.each([
    {
      name: '同じ記号を順に対応づける',
      source: [
        ['OP', 10],
        ['A', 100],
        ['B', 700],
      ],
      target: [
        ['OP', 12],
        ['A', 110],
        ['B', 710],
      ],
      want: [
        { source: 10, target: 12 },
        { source: 100, target: 110 },
        { source: 700, target: 710 },
      ],
    },
    {
      name: '誤った区切りは、ずれの近いものを選ぶ (TID 8027 の実データ)',
      source: [
        ['A', 128],
        ['A', 323],
        ['A', 727],
        ['B', 805],
        ['ED', 1460],
      ],
      target: [
        ['A', 324],
        ['B', 801],
      ],
      want: [
        { source: 323, target: 324 },
        { source: 805, target: 801 },
      ],
    },
    {
      name: '片方にしかない区切りは飛ばす',
      source: [
        ['A', 294],
        ['B', 807],
        ['ED', 1464],
      ],
      target: [
        ['OP', 136],
        ['A', 294],
        ['B', 806],
      ],
      want: [
        { source: 294, target: 294 },
        { source: 807, target: 806 },
      ],
    },
    {
      name: '共通の区切りが 1 つだけなら対応づけない',
      source: [['OP', 143]],
      target: [
        ['OP', 143],
        ['A', 232],
        ['A', 994],
      ],
      want: [],
    },
    {
      name: 'CM の長さが違っても、上限の内なら対応づける',
      source: [
        ['A', 100],
        ['B', 700],
      ],
      target: [
        ['A', 100],
        ['B', 790],
      ],
      want: [
        { source: 100, target: 100 },
        { source: 700, target: 790 },
      ],
    },
    {
      name: 'ずれが大きく変わる対応は採らない',
      source: [
        ['A', 100],
        ['B', 700],
      ],
      target: [
        ['A', 100],
        ['B', 1200],
      ],
      want: [],
    },
    {
      name: '区切りがなければ対応づけない',
      source: [],
      target: [
        ['A', 100],
        ['B', 700],
      ],
      want: [],
    },
  ] as {
    name: string;
    source: [PartSymbol, number][];
    target: [PartSymbol, number][];
    want: { source: number; target: number }[];
  }[])('$name', ({ source, target, want }) => {
    expect(matchMarkers(markers(source), markers(target))).toEqual(want);
  });
});

describe('mapComments', () => {
  const matched = [
    { source: 100, target: 110 },
    { source: 700, target: 760 },
  ];

  test.each([
    { name: '最初の区切りより前は捨てる', time: 99, want: null },
    { name: '区間の中は、区間の始まりの差だけずらす', time: 150, want: 160 },
    { name: '次の区切りからは、次の区間のずれを使う', time: 720, want: 780 },
    { name: '最後の区切りより後も、最後のずれを使う', time: 900, want: 960 },
  ])('$name', ({ time, want }) => {
    const mapped = mapComments([comment(time)], matched);

    expect(mapped.at(0)?.time ?? null).toBe(want);
  });

  test('元の放送の区間が録画より長ければ、はみ出した分 (CM) を捨てる', () => {
    const longer = [
      { source: 100, target: 110 },
      { source: 800, target: 710 },
    ];

    expect(mapComments([comment(699), comment(701)], longer).map((c) => c.time)).toEqual([709]);
  });
});

describe('mixBroadcast', () => {
  const posts = (part: string, time: number): JikkyoComment[] => [
    comment(time, part),
    comment(time + 1, part),
  ];

  test('両方の区切りで合わせて、録画の位置のコメントにする', () => {
    const own = [...posts('A', 110), ...posts('B', 710)];
    const other = [comment(50), ...posts('A', 100), comment(150, 'hit'), ...posts('B', 700)];

    const mixed = mixBroadcast(own, other);

    expect(mixed.find((c) => c.content === 'hit')?.time).toBe(160);
    expect(mixed.some((c) => c.time === 50)).toBe(false);
  });

  test('区切りが足りなければ混ぜない', () => {
    expect(mixBroadcast(posts('A', 110), [...posts('A', 100), comment(150)])).toEqual([]);
  });
});
