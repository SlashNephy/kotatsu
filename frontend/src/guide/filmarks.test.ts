import { describe, expect, it } from 'vitest';

import { filmarksSearchURL } from './filmarks';

describe('filmarksSearchURL', () => {
  it.each([
    {
      name: 'drama',
      genres: [{ lv1: 0x3, lv2: 0x0 }],
      title: '豊臣兄弟!',
      anime: false,
      want: 'https://filmarks.com/search/dramas?q=%E8%B1%8A%E8%87%A3%E5%85%84%E5%BC%9F!',
    },
    {
      name: 'movie',
      genres: [{ lv1: 0x6, lv2: 0x1 }],
      title: 'A B',
      anime: false,
      want: 'https://filmarks.com/search/movies?q=A%20B',
    },
    // 先に並ぶジャンルで探す。
    {
      name: 'first genre wins',
      genres: [
        { lv1: 0x0, lv2: 0x0 },
        { lv1: 0x6, lv2: 0x0 },
        { lv1: 0x3, lv2: 0x0 },
      ],
      title: 'x',
      anime: false,
      want: 'https://filmarks.com/search/movies?q=x',
    },
    // アニメの映画が先に並べば、後ろのジャンルがドラマでも出さない。
    {
      name: 'anime movie before drama',
      genres: [
        { lv1: 0x6, lv2: 0x2 },
        { lv1: 0x3, lv2: 0x0 },
      ],
      title: 'x',
      anime: false,
      want: null,
    },
    // アニメの映画と、しょぼいカレンダーの話に結びついた番組には出さない。
    { name: 'anime movie', genres: [{ lv1: 0x6, lv2: 0x2 }], title: 'x', anime: false, want: null },
    { name: 'linked anime', genres: [{ lv1: 0x3, lv2: 0x0 }], title: 'x', anime: true, want: null },
    { name: 'other genre', genres: [{ lv1: 0x7, lv2: 0x0 }], title: 'x', anime: false, want: null },
    { name: 'no genre', genres: [], title: 'x', anime: false, want: null },
    {
      name: 'empty title',
      genres: [{ lv1: 0x3, lv2: 0x0 }],
      title: '  ',
      anime: false,
      want: null,
    },
  ])('$name', ({ genres, title, anime, want }) => {
    expect(filmarksSearchURL(genres, title, anime)).toBe(want);
  });
});
