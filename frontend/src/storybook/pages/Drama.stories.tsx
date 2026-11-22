import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import { broadcastDay } from '../../guide/layout';
import { GraphQLRequestError } from '../../relay/requestError';
import { asViewer, merge } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 視聴管理のドラマ (/drama) の画面 (#2056)。作品がないとき、作品のカード (放送の時期、再放送、作品名の分からない作品、見た話、
// 次の話、見終えた作品、録画のある話)、視聴者のとき、絞り込みで当たらないとき、操作に失敗したときの表示を並べる。
// デモは TMDB と連携しないので、既定は連携を無効にした画面になる。
const meta = {
  title: 'ページ/ドラマ',
  component: PageStory,
  args: { url: '/drama' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const hour = 3_600_000;
const at = (time: number) => new Date(time).toISOString();

const series = (tmdbId: number, name: string | null, originalName: string | null = null) => ({
  tmdbId,
  name,
  originalName,
  url: `https://tv.example.com/tv/${tmdbId}`,
});

const nextBroadcast = (
  id: string,
  start: number,
  fields: Partial<{
    seasonNumber: number | null;
    episodeNumber: number | null;
    rerun: boolean;
    reservation: { enabled: boolean } | null;
  }> = {},
) => ({
  seasonNumber: fields.seasonNumber === undefined ? 1 : fields.seasonNumber,
  episodeNumber: fields.episodeNumber === undefined ? 4 : fields.episodeNumber,
  rerun: fields.rerun ?? false,
  last: false,
  program: {
    id: `Program:${id}`,
    name: `ドラマ「${id}」`,
    startAt: at(start),
    endAt: at(start + hour),
    reservation: fields.reservation ?? null,
    service: { name: 'デモ総合', logoUrl: null, channelType: { type: 'GR' } },
  },
});

const recorded = (from: number, to: number, watched: boolean | null, recordingCount = 1) =>
  Array.from({ length: to - from + 1 }, (_, index) => ({
    seasonNumber: 1,
    episodeNumber: from + index,
    name: `第${from + index}話の題`,
    recordingCount,
    watched,
    recording: { id: `Recording:drama-${from + index}` },
  }));

const base = {
  status: 'WATCHING',
  lastWatched: null,
  nextEpisode: null,
  nextBroadcast: null,
  recentBroadcast: null,
  recordedEpisodes: [],
};

// worksAt は、now を今として、作品を返す。放送の時期は、番組表と同じ放送日 (4 時から翌 4 時) で決まるので、日の区切りから決める。
const worksAt = (now: number) => {
  const today = broadcastDay(now);
  const yesterday = broadcastDay(today.from - 1);

  return [
    // 今日 (放送中)。見た話と次の話がある。予約済み。まだ見ていない録画が 6 話を超えるので、残りを数で示す。2 回録った話もある。
    {
      ...base,
      id: 'DramaWork:1',
      series: series(1001, '冬の灯'),
      lastWatched: { seasonNumber: 1, episodeNumber: 3, name: '雪の朝' },
      nextEpisode: {
        seasonNumber: 1,
        episodeNumber: 4,
        name: '春を待つ',
        url: 'https://tv.example.com/tv/1001/season/1/episode/4',
      },
      nextBroadcast: nextBroadcast('冬の灯', now - 10 * 60_000, {
        reservation: { enabled: true },
      }),
      recordedEpisodes: [...recorded(4, 4, false, 2), ...recorded(5, 10, false)],
    },
    // 明日。日本語の作品名がないので、原題で出す。まだ見ていない。次の放送は再放送で、話が分からない。予約を無効にしている。
    // 見たかが分からない録画 (見たかを示さない)。
    {
      ...base,
      id: 'DramaWork:2',
      series: series(1002, null, 'The Harbor'),
      nextEpisode: { seasonNumber: 0, episodeNumber: 1, name: null, url: '' },
      nextBroadcast: nextBroadcast('The Harbor', today.to + 2 * hour, {
        seasonNumber: null,
        episodeNumber: null,
        rerun: true,
        reservation: { enabled: false },
      }),
      recordedEpisodes: recorded(1, 2, null),
    },
    // それより後 (バッジなし)。作品名が分からない。予約していない。
    {
      ...base,
      id: 'DramaWork:3',
      series: series(1003, null),
      nextEpisode: { seasonNumber: 2, episodeNumber: 1, name: null, url: '' },
      nextBroadcast: nextBroadcast('名もなき作品', today.to + 30 * hour, {
        seasonNumber: 2,
        episodeNumber: 1,
      }),
    },
    // 昨日。これからの放送がない。見終えた (次の話がない)。
    {
      ...base,
      id: 'DramaWork:4',
      series: series(1004, '港町の探偵'),
      lastWatched: { seasonNumber: 1, episodeNumber: 10, name: null },
      recentBroadcast: { startAt: at(yesterday.from + hour), last: false },
    },
    // 終了 (最も新しい放送が最終話)。見たいリストの作品で、見た話も次の話も分からない。
    {
      ...base,
      id: 'DramaWork:5',
      status: 'WANNA_WATCH',
      series: series(1005, '古都の四季'),
      recentBroadcast: { startAt: at(yesterday.from - 48 * hour), last: true },
    },
  ];
};

const library = () => ({ enabled: true, works: worksAt(Date.now()) });

// signedOut は、作品の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'DramaLibraryQuery') {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

const click =
  (name: string): Story['play'] =>
  async ({ canvas }) => {
    const [first] = await canvas.findAllByRole('button', { name });
    if (first !== undefined) {
      await userEvent.click(first);
    }
  };

// 既定。管理者が TMDB との連携を有効にしていないとき。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { dramaLibrary: failing() } } };

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// 連携は有効だが、見ている作品がないとき (足し方の案内を出す)。
export const Empty: Story = {
  args: { overrides: { dramaLibrary: () => ({ enabled: true, works: [] }) } },
};

// 見ている作品 (既定の絞り込み)。
export const Watching: Story = { args: { overrides: { dramaLibrary: library } } };

// すべての作品。放送の時期 (今日、昨日、明日、終了、それより後) のそれぞれの作品を並べる。
export const All: Story = {
  args: { url: '/drama?status=all', overrides: { dramaLibrary: library } },
};

// 絞り込みに当たる作品がないとき (作品はあるので、足し方の案内は出さない)。
export const NoMatch: Story = {
  args: { url: '/drama?period=unknown', overrides: { dramaLibrary: library } },
};

// 視聴者 (予約できない) のとき。予約のボタンを出さない。
export const AsViewer: Story = {
  args: { overrides: merge(() => ({ dramaLibrary: library }), asViewer('VIEWER')) },
};

// 予約できなかったとき。
export const ReserveFailed: Story = {
  args: { overrides: { dramaLibrary: library, createProgramReservation: failing() } },
  play: click('予約する'),
};

// 見たことにできなかったとき。
export const MarkFailed: Story = {
  args: { overrides: { dramaLibrary: library, setDramaEpisodeWatched: failing() } },
  play: click('シーズン1 第4話を見たことにする'),
};

// 視聴管理の状態のメニューを開いたとき (状態の切り替えと、視聴管理から外す)。
export const StatusMenu: Story = {
  args: { overrides: { dramaLibrary: library } },
  play: click('見ている'),
};

// 視聴管理の状態を変えられなかったとき。
export const StatusFailed: Story = {
  args: { overrides: { dramaLibrary: library, setDramaWatchStatus: failing() } },
  play: async (context) => {
    await click('見ている')?.(context);
    await userEvent.click(await within(document.body).findByRole('menuitem', { name: '見たい' }));
  },
};
