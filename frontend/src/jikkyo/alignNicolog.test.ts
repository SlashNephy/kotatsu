import { describe, expect, test } from 'vitest';

import { alignNicolog } from './alignNicolog';
import type { JikkyoComment } from './comments';
import type { PartMarker, PartSymbol } from './partMarkers';

const markers = (rows: [PartSymbol, number][]): PartMarker[] =>
  rows.map(([part, time]) => ({ part, time, noCmBefore: false }));
const comment = (time: number): JikkyoComment => ({ time, content: 'c', mail: '' });
const sections = [
  { from: 300, to: 360 },
  { from: 900, to: 960 },
];

describe('alignNicolog', () => {
  test.each([
    { name: 'OP の位置を合わせる', time: 100, want: 120 },
    { name: 'CM を挟んだ後ろは、CM の分だけずらす', time: 400, want: 480 },
    { name: '2 つの CM の後ろは、両方の分だけずらす', time: 1000, want: 1140 },
  ])('$name', ({ time, want }) => {
    const aligned = alignNicolog(
      [comment(time)],
      markers([['OP', 100]]),
      markers([['OP', 120]]),
      sections,
    );

    expect(aligned.map((c) => c.time)).toEqual([want]);
  });

  test('OP より前のコメントは捨てる', () => {
    expect(
      alignNicolog([comment(99)], markers([['OP', 100]]), markers([['OP', 120]]), sections),
    ).toEqual([]);
  });

  test('録画の OP が CM の後ろなら、CM を除いた時刻で合わせる', () => {
    const aligned = alignNicolog(
      [comment(200)],
      markers([['OP', 100]]),
      markers([['OP', 400]]),
      sections,
    );

    expect(aligned.map((c) => c.time)).toEqual([500]);
  });

  test.each([
    { name: '上映会に OP がない', nicolog: [], own: [['OP', 120]], cm: sections },
    { name: '録画に OP がない', nicolog: [['OP', 100]], own: [], cm: sections },
    { name: 'CM の区間がない', nicolog: [['OP', 100]], own: [['OP', 120]], cm: [] },
    { name: '録画の OP が CM の中', nicolog: [['OP', 100]], own: [['OP', 330]], cm: sections },
  ] as {
    name: string;
    nicolog: [PartSymbol, number][];
    own: [PartSymbol, number][];
    cm: typeof sections;
  }[])('$name なら混ぜない', ({ nicolog, own, cm }) => {
    expect(alignNicolog([comment(200)], markers(nicolog), markers(own), cm)).toEqual([]);
  });
});
