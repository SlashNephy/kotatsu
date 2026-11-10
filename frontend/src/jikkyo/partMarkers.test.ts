import { describe, expect, test } from 'vitest';

import type { JikkyoComment } from './comments';
import { findPartMarkers } from './partMarkers';

// comments は、[時刻, 本文] の並びから過去ログを作る。
const comments = (rows: [number, string][]): JikkyoComment[] =>
  rows.map(([time, content]) => ({ time, content, mail: '' }));

describe('findPartMarkers', () => {
  test.each([
    {
      name: '固まった同じ記号の最初の時刻を区切りにする',
      rows: [
        [100, 'A'],
        [103, 'A'],
        [106, 'A'],
      ],
      want: [{ part: 'A', time: 100, noCmBefore: false }],
    },
    {
      name: '1 件だけの書き込みは区切りにしない',
      rows: [[100, 'ED']],
      want: [],
    },
    {
      name: '8 秒より離れた 2 件は固まりにしない',
      rows: [
        [100, 'B'],
        [108.5, 'B'],
      ],
      want: [],
    },
    {
      name: '8 秒以内の 2 件は固まりにする',
      rows: [
        [100, 'B'],
        [108, 'B'],
      ],
      want: [{ part: 'B', time: 100, noCmBefore: false }],
    },
    {
      name: '全角と半角は同じ記号にする',
      rows: [
        [100, 'Ａ'],
        [101, 'A'],
      ],
      want: [{ part: 'A', time: 100, noCmBefore: false }],
    },
    {
      name: '2 文字の記号も全角と半角は同じ記号にする',
      rows: [
        [100, 'ＯＰ'],
        [101, 'OP'],
      ],
      want: [{ part: 'OP', time: 100, noCmBefore: false }],
    },
    {
      name: '前後の空白は無視する',
      rows: [
        [100, ' C '],
        [101, 'C'],
      ],
      want: [{ part: 'C', time: 100, noCmBefore: false }],
    },
    {
      name: '小文字は記号にしない',
      rows: [
        [100, 'b'],
        [101, 'b'],
      ],
      want: [],
    },
    {
      name: 'ほかの言葉は記号にしない',
      rows: [
        [100, 'Aパート'],
        [101, 'Aパート'],
      ],
      want: [],
    },
    {
      name: '違う記号が挟まっても、同じ記号どうしで固まりを見る',
      rows: [
        [100, 'OP'],
        [101, 'A'],
        [102, 'OP'],
      ],
      want: [{ part: 'OP', time: 100, noCmBefore: false }],
    },
    {
      name: '60 秒以内の同じ記号の固まりは 1 つにする',
      rows: [
        [100, 'B'],
        [101, 'B'],
        [130, 'B'],
        [131, 'B'],
      ],
      want: [{ part: 'B', time: 100, noCmBefore: false }],
    },
    {
      name: '60 秒より離れた同じ記号の固まりは別の区切りにする',
      rows: [
        [100, 'OP'],
        [101, 'OP'],
        [1900, 'OP'],
        [1901, 'OP'],
      ],
      want: [
        { part: 'OP', time: 100, noCmBefore: false },
        { part: 'OP', time: 1900, noCmBefore: false },
      ],
    },
    {
      name: 'OP の開始から 120 秒以内の A は、前に CM を挟まない',
      rows: [
        [100, 'OP'],
        [101, 'OP'],
        [220, 'A'],
        [221, 'A'],
      ],
      want: [
        { part: 'OP', time: 100, noCmBefore: false },
        { part: 'A', time: 220, noCmBefore: true },
      ],
    },
    {
      name: 'OP の開始から 120 秒を超えた A は、前に CM を挟む',
      rows: [
        [100, 'OP'],
        [101, 'OP'],
        [260, 'A'],
        [261, 'A'],
      ],
      want: [
        { part: 'OP', time: 100, noCmBefore: false },
        { part: 'A', time: 260, noCmBefore: false },
      ],
    },
    {
      name: 'B は OP の直後でも CM の判定をしない',
      rows: [
        [100, 'OP'],
        [101, 'OP'],
        [150, 'B'],
        [151, 'B'],
      ],
      want: [
        { part: 'OP', time: 100, noCmBefore: false },
        { part: 'B', time: 150, noCmBefore: false },
      ],
    },
  ] as { name: string; rows: [number, string][]; want: unknown[] }[])('$name', ({ rows, want }) => {
    expect(findPartMarkers(comments(rows))).toEqual(want);
  });
});
