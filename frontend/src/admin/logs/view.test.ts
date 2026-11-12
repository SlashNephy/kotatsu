import { describe, expect, test } from 'vitest';

import { filterLines, mergeLines, type ViewLine } from './view';

const kotatsu = {
  lines: [
    { time: 1, level: 'INFO', inferred: false, text: 'started' },
    { time: 3, level: 'ERROR', inferred: false, text: 'Recording FAILED' },
  ] as const,
};
const tuner = {
  lines: [
    { time: 2, level: 'WARN', inferred: true, text: 'tuner warning' },
    { time: 3, level: 'INFO', inferred: true, text: 'same time' },
  ] as const,
};

describe('mergeLines', () => {
  test('出どころを混ぜて、時刻の新しい順に並べる', () => {
    expect(mergeLines([kotatsu, tuner]).map((line) => [line.source, line.text])).toEqual([
      [0, 'Recording FAILED'],
      [1, 'same time'],
      [1, 'tuner warning'],
      [0, 'started'],
    ]);
  });
});

describe('filterLines', () => {
  const lines: ViewLine[] = mergeLines([kotatsu, tuner]);
  const texts = (filtered: ViewLine[]) => filtered.map((line) => line.text);

  test.each([
    {
      name: '絞らなければ全部',
      hidden: [],
      problemsOnly: false,
      search: '',
      want: ['Recording FAILED', 'same time', 'tuner warning', 'started'],
    },
    {
      name: '出どころの表示を切る',
      hidden: [1],
      problemsOnly: false,
      search: '',
      want: ['Recording FAILED', 'started'],
    },
    {
      name: '警告とエラーだけ (推し量った重さも含む)',
      hidden: [],
      problemsOnly: true,
      search: '',
      want: ['Recording FAILED', 'tuner warning'],
    },
    {
      name: '大文字と小文字を区別しない部分一致',
      hidden: [],
      problemsOnly: false,
      search: ' failed ',
      want: ['Recording FAILED'],
    },
    {
      name: '組み合わせ',
      hidden: [0],
      problemsOnly: true,
      search: 'TUNER',
      want: ['tuner warning'],
    },
  ])('$name', ({ hidden, problemsOnly, search, want }) => {
    expect(texts(filterLines(lines, { hidden: new Set(hidden), problemsOnly, search }))).toEqual(
      want,
    );
  });
});
