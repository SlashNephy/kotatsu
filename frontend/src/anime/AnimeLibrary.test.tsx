import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createDemoEnvironment, renderWithRouter } from '../test/route';
import { AnimeLibrary } from './AnimeLibrary';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const workImage = 'https://example.com/work.jpg';
const thumbnail = '/media/thumbnails/1.jpg';

// library は、作品の画像 imageUrl と、録画の代表画像を 1 つ持つ作品だけのライブラリ。
function library(imageUrl: string | null) {
  return {
    enabled: true,
    imports: [
      { destination: 'ANNICT', importedAt: '2026-11-11T00:00:00Z', error: null, entryCount: 1 },
    ],
    works: [
      {
        id: 'AnimeLibraryWork:1',
        tid: 1,
        title: 'こたつの作品',
        imageUrl,
        status: 'WATCHING',
        progress: 0,
        episodesCount: 12,
        seasonYear: 2026,
        season: null,
        sources: [],
        nextEpisode: null,
        broadcasts: [],
        recentBroadcasts: [],
        recordedEpisodes: [
          {
            countStart: 1,
            countEnd: 1,
            subtitle: null,
            recordingCount: 1,
            watched: false,
            recording: { __typename: 'Recording', id: 'Recording:1', thumbnailUrl: thumbnail },
          },
        ],
      },
    ],
  };
}

async function renderLibrary(imageUrl: string | null) {
  const environment = createDemoEnvironment({
    animeLibrary: () => library(imageUrl),
    animeWishlist: () => ({ works: [] }),
  });
  const { container } = await renderWithRouter(
    <AnimeLibrary
      filters={{
        status: 'all',
        season: undefined,
        excludeReruns: false,
        period: undefined,
        weekday: undefined,
      }}
      streaming={[]}
    />,
    environment,
  );
  await screen.findByText('こたつの作品');

  return container;
}

// failImage は、作品のカードの画像の読み込みを失敗させ、失敗の後に出した画像の src を返す。
async function failImage(container: HTMLElement): Promise<string | null> {
  const image = container.querySelector('section img');
  if (image === null) {
    return null;
  }
  await act(async () => fireEvent.error(image));

  return container.querySelector('section img')?.getAttribute('src') ?? null;
}

describe('AnimeLibrary の作品の画像 (#1433)', () => {
  test('作品の画像が読めなければ、録画の代表画像に替え、それも読めなければ画像の枠を消す', async () => {
    const container = await renderLibrary(workImage);
    expect(container.querySelector('section img')?.getAttribute('src')).toBe(workImage);

    expect(await failImage(container)).toBe(thumbnail);
    expect(await failImage(container)).toBeNull();
  });

  test('作品の画像がなく、録画の代表画像も読めなければ、画像の枠を消す', async () => {
    const container = await renderLibrary(null);
    expect(container.querySelector('section img')?.getAttribute('src')).toBe(thumbnail);

    expect(await failImage(container)).toBeNull();
  });
});
