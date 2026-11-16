import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { RelayEnvironmentProvider } from 'react-relay';
import { afterEach, describe, expect, test } from 'vitest';

import i18next from '../i18n';
import { createTestEnvironment } from '../test/route';
import {
  DramaEpisode,
  dramaEpisodeLabel,
  parseTMDbSeriesId,
  TMDB_NOTICE,
  type DramaEpisodeItem,
} from './DramaEpisode';

const t = i18next.t;

afterEach(() => {
  cleanup();
});

// episode は、もうパパ！のシーズン 1 第 7 話に結びついた番組。
function episode(overrides: Partial<DramaEpisodeItem> = {}): DramaEpisodeItem {
  return {
    series: {
      tmdbId: 331304,
      name: 'もうパパ！',
      originalName: 'もうパパ！',
      url: 'https://www.themoviedb.org/tv/331304',
    },
    seriesSource: 'SEARCH',
    seasonNumber: 1,
    episodeNumber: 7,
    episodeName: '第7話の名前',
    url: 'https://www.themoviedb.org/tv/331304/season/1/episode/7',
    ...overrides,
  };
}

function renderEpisode(item: DramaEpisodeItem | null, canOverride: boolean) {
  return render(
    <RelayEnvironmentProvider environment={createTestEnvironment(() => undefined)}>
      <MantineProvider>
        <DramaEpisode episode={item} programName="もうパパ！　＃７" canOverride={canOverride} />
      </MantineProvider>
    </RelayEnvironmentProvider>,
  );
}

describe('dramaEpisodeLabel', () => {
  test.each([
    { name: 'シーズンと話を並べる', season: 7, number: 7, want: 'シーズン7 第7話' },
    { name: 'シーズン 0 は特別編', season: 0, number: 1, want: '特別編 第1話' },
    { name: '話がなければ出さない', season: null, number: null, want: null },
  ])('$name', ({ season, number, want }) => {
    expect(dramaEpisodeLabel(t, episode({ seasonNumber: season, episodeNumber: number }))).toBe(
      want,
    );
  });
});

describe('parseTMDbSeriesId', () => {
  test.each([
    { name: 'ID', value: '331304', want: 331304 },
    {
      name: '作品のページの URL',
      value: 'https://www.themoviedb.org/tv/331304-mou-papa',
      want: 331304,
    },
    {
      name: '話のページの URL',
      value: 'https://www.themoviedb.org/tv/202396/season/7/episode/7',
      want: 202396,
    },
    { name: '映画の URL は読まない', value: 'https://www.themoviedb.org/movie/603', want: null },
    { name: '0 は ID ではない', value: '0', want: null },
    { name: '大きすぎる', value: '99999999999', want: null },
    { name: '空', value: ' ', want: null },
  ])('$name', ({ value, want }) => {
    expect(parseTMDbSeriesId(value)).toBe(want);
  });
});

describe('DramaEpisode', () => {
  test('作品名、シーズンと話、話の名前、TMDb の話のページと、帰属の表示を出す', () => {
    renderEpisode(episode(), false);

    expect(screen.getByText('もうパパ！')).toBeDefined();
    expect(screen.getByText('シーズン1 第7話')).toBeDefined();
    expect(screen.getByText('「第7話の名前」')).toBeDefined();
    const link = screen.getByRole('link', { name: 'TMDb' });
    expect(link.getAttribute('href')).toBe(
      'https://www.themoviedb.org/tv/331304/season/1/episode/7',
    );
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    expect(screen.getByText(TMDB_NOTICE)).toBeDefined();
    expect(screen.getByRole('img', { name: 'TMDB' })).toBeDefined();
    expect(screen.queryByRole('button', { name: '作品を直す' })).toBeNull();
  });

  test('話を決められなければ、作品のページだけを出す', () => {
    renderEpisode(
      episode({
        seasonNumber: null,
        episodeNumber: null,
        episodeName: null,
        url: 'https://www.themoviedb.org/tv/331304',
      }),
      false,
    );

    expect(screen.queryByText(/シーズン/)).toBeNull();
    expect(screen.getByRole('link', { name: 'TMDb' }).getAttribute('href')).toBe(
      'https://www.themoviedb.org/tv/331304',
    );
  });

  test('管理者には、作品を直す操作を出し、管理者が指定した作品には印を付ける', () => {
    renderEpisode(episode({ seriesSource: 'OVERRIDE' }), true);

    expect(screen.getByRole('button', { name: '作品を直す' })).toBeDefined();
    expect(screen.getByText('管理者が指定')).toBeDefined();
  });

  test('結びついていなければ、何も出さない', () => {
    const { container } = renderEpisode(null, true);

    expect(container.querySelector('[data-testid="drama-episode"]')).toBeNull();
  });
});
