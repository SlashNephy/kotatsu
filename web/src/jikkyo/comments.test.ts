import { describe, expect, test } from 'vitest';

import { shouldDraw, toFormatted } from './comments';

describe('shouldDraw', () => {
  test.each([
    { name: 'ふつうのコメント', content: 'こんにちは', want: true },
    { name: '空のコメント', content: '  ', want: false },
    { name: '運営のコマンド', content: '/nicoad {"totalAdPoint":100}', want: false },
  ])('$name', ({ content, want }) => {
    expect(shouldDraw({ time: 0, content, mail: '184' })).toBe(want);
  });
});

describe('toFormatted', () => {
  test('時刻を 1/100 秒に丸め、コマンドを空白で分ける', () => {
    expect(toFormatted({ time: 12.345, content: 'わこつ', mail: '184  shita big ' }, 7)).toEqual({
      id: 7,
      vpos: 1235,
      content: 'わこつ',
      mail: ['184', 'shita', 'big'],
    });
  });
});
