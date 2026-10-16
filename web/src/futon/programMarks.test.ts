import { describe, expect, test } from 'vitest';

import { splitProgramMarks } from './programMarks';

describe('splitProgramMarks', () => {
  test.each([
    { name: '囲み文字がない', input: 'ニュース', want: { name: 'ニュース', marks: [] } },
    {
      name: '末尾の囲み文字を、中身にして取り出す',
      input: '科捜研の女１６　＃５🈖🈑🈞',
      want: { name: '科捜研の女１６　＃５', marks: ['解', '字', '再'] },
    },
    {
      name: '途中や先頭の囲み文字も取り出し、残った空白を詰める',
      input: '🈟 ドラマ 🈑 第1話',
      want: { name: 'ドラマ 第1話', marks: ['新', '字'] },
    },
    {
      name: '英字の囲み文字は、略号にする',
      input: '映画🅊🄿',
      want: { name: '映画', marks: ['HV', 'P'] },
    },
    { name: '同じ囲み文字は 1 つにする', input: '🈑番組🈑', want: { name: '番組', marks: ['字'] } },
    {
      name: '4K や 5.1 などの囲み文字も取り出す',
      input: '世界遺産🆞🆠🆧',
      want: { name: '世界遺産', marks: ['4K', '5.1', 'HDR'] },
    },
    {
      name: '鍵の囲み文字を取り出す',
      input: '映画⚿',
      want: { name: '映画', marks: ['鍵'] },
    },
    {
      name: '🈀 (ほか) は、番組名に「ほか」として残す',
      input: '出演：山田🈀🈑',
      want: { name: '出演：山田ほか', marks: ['字'] },
    },
    {
      name: '丸数字は番組名の一部として残す',
      input: '講座①',
      want: { name: '講座①', marks: [] },
    },
  ])('$name', ({ input, want }) => {
    expect(splitProgramMarks(input)).toEqual(want);
  });
});
