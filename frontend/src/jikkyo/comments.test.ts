import { describe, expect, test } from 'vitest';

import {
  compileMute,
  defaultCommentSettings,
  shouldDraw,
  toFormatted,
  type CommentSettings,
} from './comments';

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

describe('toFormatted の速さ', () => {
  test.each([
    { name: '倍率が 1 なら、秒数を足さない', mail: '184', speed: 1, want: ['184'] },
    { name: '速くすると、出しておく秒数を縮める', mail: '184', speed: 2, want: ['184', '@1.50'] },
    { name: '遅くすると、出しておく秒数を延ばす', mail: '', speed: 0.5, want: ['@6.00'] },
    {
      name: 'コメントが秒数を指定していれば、それに従う',
      mail: '184 @5',
      speed: 2,
      want: ['184', '@5'],
    },
  ])('$name', ({ mail, speed, want }) => {
    expect(toFormatted({ time: 0, content: 'わこつ', mail }, 0, speed).mail).toEqual(want);
  });
});

describe('compileMute', () => {
  const settings = (
    mutedKeywords: { text: string; match: string }[],
    mutedUsers: string[] = [],
  ): CommentSettings => ({ ...defaultCommentSettings, mutedKeywords, mutedUsers });
  test.each([
    {
      name: '含む',
      keywords: [{ text: 'ネタバレ', match: 'PARTIAL' }],
      content: 'これはネタバレです',
      want: true,
    },
    {
      name: '含まない',
      keywords: [{ text: 'ネタバレ', match: 'PARTIAL' }],
      content: 'わこつ',
      want: false,
    },
    {
      name: '全角と半角、大文字と小文字をそろえる',
      keywords: [{ text: 'ｗｗｗ', match: 'PARTIAL' }],
      content: 'WWW',
      want: true,
    },
    {
      name: 'で始まる',
      keywords: [{ text: '草', match: 'PREFIX' }],
      content: '草生える',
      want: true,
    },
    {
      name: 'で始まらない',
      keywords: [{ text: '草', match: 'PREFIX' }],
      content: '大草原',
      want: false,
    },
    {
      name: 'で終わる',
      keywords: [{ text: 'ｗ', match: 'SUFFIX' }],
      content: 'すごいw',
      want: true,
    },
    {
      name: '一致する',
      keywords: [{ text: '8888', match: 'EXACT' }],
      content: ' 8888 ',
      want: true,
    },
    {
      name: '一致しない',
      keywords: [{ text: '8888', match: 'EXACT' }],
      content: '88888',
      want: false,
    },
    { name: '正規表現', keywords: [{ text: '^w+$', match: 'REGEX' }], content: 'WWWW', want: true },
    {
      name: '読めない正規表現は無視する',
      keywords: [{ text: '(', match: 'REGEX' }],
      content: '(',
      want: false,
    },
  ])('$name', ({ keywords, content, want }) => {
    expect(compileMute(settings(keywords))({ time: 0, content, mail: '' })).toBe(want);
  });

  test('ユーザー ID でミュートし、ID の分からないコメントはミュートしない', () => {
    const muted = compileMute(settings([], ['a:x']));
    expect(muted({ time: 0, content: 'わこつ', mail: '', userId: 'a:x' })).toBe(true);
    expect(muted({ time: 0, content: 'わこつ', mail: '', userId: 'a:y' })).toBe(false);
    expect(muted({ time: 0, content: 'わこつ', mail: '' })).toBe(false);
  });
});
