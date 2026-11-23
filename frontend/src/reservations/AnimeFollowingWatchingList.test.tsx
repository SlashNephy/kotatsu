import { cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { AnimeFollowingWatchingList } from './AnimeFollowingWatchingList';

afterEach(() => {
  cleanup();
});

// connected は、Annict と連携した記録先の様子。
const connected = {
  destinations: [
    { destination: 'ANNICT', connection: { connectedAt: '2026-11-20T00:00:00Z' } },
    { destination: 'ANILIST', connection: null },
  ],
};

// work は、フォロー中の人が見ている作品。
function work(watchers: number) {
  return {
    tid: 6776,
    title: '灯台の地図',
    reservedBroadcasts: 0,
    covered: false,
    watchers: Array.from({ length: watchers }, (_, i) => ({
      destination: 'ANNICT',
      username: `user${i}`,
      name: `ユーザー${i}`,
      avatarUrl: null,
      profileUrl: `https://annict.com/@user${i}`,
    })),
    broadcasts: [
      {
        preferred: true,
        program: {
          id: 'Program:1',
          name: '灯台の地図 #5',
          startAt: '2026-11-23T12:00:00Z',
          endAt: '2026-11-23T12:30:00Z',
          service: {
            id: 'Service:1',
            name: 'こたつテスト1',
            logoUrl: null,
            channelType: { id: 'ChannelType:GR', type: 'GR' },
          },
        },
      },
    ],
  };
}

describe('フォロー中の人が見ている作品 (#1968)', () => {
  test('見ている人の数と、予約とルールの操作を出す', async () => {
    const environment = createTestEnvironment((request) =>
      request.name === 'AnimeFollowingWatchingListQuery'
        ? {
            data: {
              viewer: { id: 'User:1', role: 'MEMBER' },
              animeRecordSettings: connected,
              animeFollowingWatching: { enabled: true, works: [work(3)] },
            },
          }
        : undefined,
    );
    await renderWithRouter(<AnimeFollowingWatchingList />, environment);

    expect(await screen.findByRole('heading', { name: '灯台の地図' })).toBeTruthy();
    expect(screen.getByText('3人が見ています')).toBeTruthy();
    expect(screen.getAllByRole('link', { name: /ユーザー\d（Annict）/ })).toHaveLength(3);
    expect(screen.getByRole('button', { name: 'ルールを作る' })).toBeTruthy();
    expect(screen.getByRole('button', { name: '予約する' })).toBeTruthy();
  });

  test('視聴者には、ルールと予約のボタンを出さない', async () => {
    const environment = createTestEnvironment(() => ({
      data: {
        viewer: { id: 'User:1', role: 'VIEWER' },
        animeRecordSettings: connected,
        animeFollowingWatching: { enabled: true, works: [work(1)] },
      },
    }));
    await renderWithRouter(<AnimeFollowingWatchingList />, environment);

    expect(await screen.findByText('1人が見ています')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'ルールを作る' })).toBeNull();
    expect(screen.queryByRole('button', { name: '予約する' })).toBeNull();
  });

  test('連携していなければ、連携を勧める', async () => {
    const environment = createTestEnvironment(() => ({
      data: {
        viewer: { id: 'User:1', role: 'MEMBER' },
        animeRecordSettings: { destinations: [{ destination: 'ANNICT', connection: null }] },
        animeFollowingWatching: { enabled: true, works: [] },
      },
    }));
    await renderWithRouter(<AnimeFollowingWatchingList />, environment);

    expect(
      await screen.findByText(
        'AnnictかAniListと連携すると、フォローしている人が見ている作品の放送がここに出ます',
      ),
    ).toBeTruthy();
  });

  test('ルールを作ると、作ったルールへのリンクを出す', async () => {
    const environment = createTestEnvironment((request) =>
      request.name === 'AnimeFollowingWatchingListCreateRuleMutation'
        ? { data: { createAnimeFollowingWatchingRule: { id: 'Rule:1', name: '灯台の地図' } } }
        : {
            data: {
              viewer: { id: 'User:1', role: 'MEMBER' },
              animeRecordSettings: connected,
              animeFollowingWatching: { enabled: true, works: [work(2)] },
            },
          },
    );
    await renderWithRouter(<AnimeFollowingWatchingList />, environment);

    fireEvent.click(await screen.findByRole('button', { name: 'ルールを作る' }));

    expect(await screen.findByRole('link', { name: 'ルールを開く' })).toBeTruthy();
  });
});
