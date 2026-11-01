import { describe, expect, test } from 'vitest';

import i18next from '../i18n';
import { episodeLabel, episodeLinks } from './AnimeEpisodes';

const t = i18next.t;

describe('episodeLabel', () => {
  test.each([
    { name: '1 話だけなら、その話数にする', from: 5, to: 5, want: '第5話' },
    { name: '話数の範囲は、始まりと終わりを並べる', from: 1, to: 2, want: '第1〜2話' },
    { name: '話数が半分の話も出す', from: 12.5, to: 12.5, want: '第12.5話' },
    { name: '話数がなければ出さない', from: null, to: null, want: null },
  ])('$name', ({ from, to, want }) => {
    expect(episodeLabel(t, from, to)).toBe(want);
  });
});

describe('episodeLinks', () => {
  test('作品のページを Annict、AniList、MyAnimeList、しょぼいカレンダーの順に並べ、同じページは重ねない', () => {
    const links = episodeLinks(t, {
      syobocalUrl: 'https://cal.syoboi.jp/tid/1',
      countStart: 1,
      countEnd: 1,
      subtitle: null,
      first: false,
      last: false,
      works: [
        {
          annictUrl: 'https://annict.com/works/2',
          anilistUrl: 'https://anilist.co/anime/3',
          malUrl: null,
        },
        // arm で 1 つの TID が複数の作品に対応するとき、同じ AniList の作品を指すことがある。
        {
          annictUrl: 'https://annict.com/works/4',
          anilistUrl: 'https://anilist.co/anime/3',
          malUrl: null,
        },
      ],
    });

    expect(links).toEqual([
      { label: 'Annict', href: 'https://annict.com/works/2' },
      { label: 'AniList', href: 'https://anilist.co/anime/3' },
      { label: 'Annict', href: 'https://annict.com/works/4' },
      { label: 'しょぼいカレンダー', href: 'https://cal.syoboi.jp/tid/1' },
    ]);
  });
});
