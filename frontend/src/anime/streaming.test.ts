import { afterEach, describe, expect, test } from 'vitest';

import {
  loadStreamingServices,
  saveStreamingServices,
  searchURL,
  streamingServices,
} from './streaming';

describe('searchURL', () => {
  test('作品名を URL に入れて探す', () => {
    expect(searchURL('danime', '葬送のフリーレン')).toBe(
      'https://animestore.docomo.ne.jp/animestore/sch_pc?searchKey=%E8%91%AC%E9%80%81%E3%81%AE%E3%83%95%E3%83%AA%E3%83%BC%E3%83%AC%E3%83%B3&vodTypeList=svod_tvod',
    );
  });

  test('記号を含む作品名も、クエリを壊さない', () => {
    expect(searchURL('youtube', 'A&B #1')).toBe(
      'https://www.youtube.com/results?search_query=A%26B%20%231',
    );
  });
});

describe('loadStreamingServices', () => {
  afterEach(() => localStorage.clear());

  test('覚えていなければ、すべてを出す', () => {
    expect(loadStreamingServices()).toEqual(streamingServices);
  });

  test('覚えた配信サービスだけを、決まった並びで出し、知らない値は捨てる', () => {
    localStorage.setItem(
      'kotatsu.anime.streamingServices',
      JSON.stringify(['youtube', 'unknown', 'abema']),
    );
    expect(loadStreamingServices()).toEqual(['abema', 'youtube']);
  });

  test('覚えたものを読み戻す', () => {
    saveStreamingServices([]);
    expect(loadStreamingServices()).toEqual([]);
  });
});
