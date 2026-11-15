import { describe, expect, it } from 'vitest';

import { streamingSearch } from './streaming';

describe('streamingSearch', () => {
  it.each([
    {
      name: 'NHK の地上波は NHK ONE で探す',
      service: { name: 'NHK総合1・東京', channelType: { type: 'GR' } },
      title: 'ブラタモリ',
      want: {
        service: 'nhk',
        url: 'https://www.web.nhk/search?query=%E3%83%96%E3%83%A9%E3%82%BF%E3%83%A2%E3%83%AA',
      },
    },
    {
      name: '全角で名乗る NHK の局も NHK ONE で探す',
      service: { name: 'ＮＨＫ　ＢＳ', channelType: { type: 'BS' } },
      title: 'A B',
      want: { service: 'nhk', url: 'https://www.web.nhk/search?query=A%20B' },
    },
    {
      name: 'NHK でない地上波は TVer で探す',
      service: { name: 'フジテレビ', channelType: { type: 'GR' } },
      title: 'A/B',
      want: { service: 'tver', url: 'https://tver.jp/search/A%2FB' },
    },
    {
      name: '民放の BS は配信の検索を出さない',
      service: { name: 'BS日テレ', channelType: { type: 'BS' } },
      title: 'x',
      want: null,
    },
    {
      name: 'CS は配信の検索を出さない',
      service: { name: 'アニマックス', channelType: { type: 'CS' } },
      title: 'x',
      want: null,
    },
    {
      name: '作品名が空なら出さない',
      service: { name: 'フジテレビ', channelType: { type: 'GR' } },
      title: '  ',
      want: null,
    },
  ])('$name', ({ service, title, want }) => {
    expect(streamingSearch(service, title)).toEqual(want);
  });
});
