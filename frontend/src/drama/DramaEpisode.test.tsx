import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
  test('作品名、シーズンと話、話の名前と、TMDB の話のページを出す', () => {
    renderEpisode(episode(), false);

    expect(screen.getByText('もうパパ！')).toBeDefined();
    expect(screen.getByText('シーズン1 第7話')).toBeDefined();
    expect(screen.getByText('「第7話の名前」')).toBeDefined();
    const link = screen.getByRole('link', { name: 'TMDB' });
    expect(link.getAttribute('href')).toBe(
      'https://www.themoviedb.org/tv/331304/season/1/episode/7',
    );
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    // 帰属の表示は、このアプリについての画面 (/about) にだけ出す。
    expect(screen.queryByText(TMDB_NOTICE)).toBeNull();
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
    expect(screen.getByRole('link', { name: 'TMDB' }).getAttribute('href')).toBe(
      'https://www.themoviedb.org/tv/331304',
    );
  });

  test('話の名前が話数だけの仮の名前なら、話の名前を出さない', () => {
    renderEpisode(episode({ episodeName: '第7話' }), false);

    expect(screen.getByText('シーズン1 第7話')).toBeDefined();
    expect(screen.queryByText('「第7話」')).toBeNull();
  });

  test('日本語の名前がなく、英語の仮の名前になった話も、話の名前を出さない', () => {
    renderEpisode(episode({ episodeName: 'Episode 7' }), false);

    expect(screen.queryByText('「Episode 7」')).toBeNull();
  });

  test('画面の言語が英語でも、日本語の仮の名前を出さない', async () => {
    await i18next.changeLanguage('en');
    try {
      renderEpisode(episode({ episodeName: '第7話' }), false);

      expect(screen.queryByText('「第7話」')).toBeNull();
    } finally {
      await i18next.changeLanguage('ja');
    }
  });

  test('話の名前が別の話数の形なら、仮の名前とみなさずに出す', () => {
    renderEpisode(episode({ episodeName: '第8話' }), false);

    expect(screen.getByText('「第8話」')).toBeDefined();
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

describe('DramaEpisode の視聴管理 (#1652)', () => {
  // renderTracked は、ログイン中のユーザーの状態を引いた詳細を出し、送った mutation の変数を requests に足す。
  function renderTracked(item: DramaEpisodeItem, requests: Record<string, unknown>[]) {
    return render(
      <RelayEnvironmentProvider
        environment={createTestEnvironment((request, variables) => {
          requests.push({ name: request.name, ...variables });

          return request.name === 'DramaWatchControlsStatusMutation'
            ? {
                data: {
                  setDramaWatchStatus: { tmdbId: 331304, watchStatus: variables['status'] ?? null },
                },
              }
            : { data: { setDramaEpisodeWatched: variables['watched'] } };
        })}
      >
        <MantineProvider>
          <DramaEpisode episode={item} programName="もうパパ！　＃７" canOverride={false} />
        </MantineProvider>
      </RelayEnvironmentProvider>,
    );
  }

  test('状態を引かなければ、視聴管理の操作を出さない', () => {
    renderEpisode(episode(), false);

    expect(screen.queryByRole('button', { name: '視聴管理に足す' })).toBeNull();
  });

  test('作品を見ている作品に足し、話を見たことにして、取り消せる', async () => {
    const user = userEvent.setup();
    const requests: Record<string, unknown>[] = [];
    renderTracked(
      episode({ watched: false, series: { ...episode().series, watchStatus: null } }),
      requests,
    );

    await user.click(screen.getByRole('button', { name: '視聴管理に足す' }));
    await user.click(await screen.findByRole('menuitem', { name: '見ている' }));
    expect(await screen.findByRole('button', { name: '見ている' })).toBeDefined();

    await user.click(screen.getByRole('button', { name: '見たことにする' }));
    expect(await screen.findByRole('button', { name: '見た' })).toBeDefined();
    await user.click(screen.getByRole('button', { name: '見た' }));
    expect(await screen.findByRole('button', { name: '見たことにする' })).toBeDefined();

    const episodeKey = { seriesId: 331304, seasonNumber: 1, episodeNumber: 7 };
    expect(requests).toEqual([
      { name: 'DramaWatchControlsStatusMutation', seriesId: 331304, status: 'WATCHING' },
      { name: 'DramaWatchControlsWatchedMutation', ...episodeKey, watched: true },
      { name: 'DramaWatchControlsWatchedMutation', ...episodeKey, watched: false },
    ]);
  });
});
