import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { broadcastDay } from '../../guide/layout';
import { GraphQLRequestError } from '../../relay/requestError';
import { asViewer, merge, withStorage } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 視聴管理のアニメ (/anime) の画面 (#2056)。連携していないとき、取り込みの状態、作品のカード (放送の時期、再放送、予約の状態、
// 次の話、録画のある話、画像の有無)、ルールの作成と予約と記録の結果、視聴者のとき、絞り込みで当たらないときの表示を並べる。
// デモは Annict と AniList につながないので、既定は連携を無効にした画面になる。
const meta = {
  title: 'ページ/アニメ',
  component: PageStory,
  args: { url: '/anime' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const hour = 3_600_000;
const at = (time: number) => new Date(time).toISOString();

const image = (color: string) =>
  `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><rect width="320" height="180" fill="${color}"/></svg>`,
  )}`;

// broadcast は、start に始まる、作品の第 count 話の放送。
const broadcast = (
  id: string,
  count: number,
  start: number,
  fields: Partial<{ rerun: boolean; reservation: { enabled: boolean } | null }> = {},
) => ({
  countStart: count,
  countEnd: count,
  rerun: fields.rerun ?? false,
  last: false,
  program: {
    id: `Program:${id}`,
    name: `アニメ「${id}」#${count}`,
    startAt: at(start),
    endAt: at(start + hour / 2),
    reservation: fields.reservation
      ? { id: `Reservation:${id}-${count}`, ...fields.reservation }
      : null,
    service: { id: 'Service:1024', name: 'デモ総合', logoUrl: null, channelType: { type: 'GR' } },
  },
});

// recorded は、第 from 話から第 to 話までの、録画のある話。
const recorded = (
  from: number,
  to: number,
  fields: Partial<{ watched: boolean | null; recordingCount: number; thumbnailUrl: string }> = {},
) =>
  Array.from({ length: to - from + 1 }, (_, index) => ({
    countStart: from + index,
    countEnd: from + index,
    subtitle: `サブタイトル ${from + index}`,
    recordingCount: fields.recordingCount ?? 1,
    watched: fields.watched === undefined ? false : fields.watched,
    recording: { id: `Recording:anime-${from + index}`, thumbnailUrl: fields.thumbnailUrl ?? null },
  }));

const base = {
  imageUrl: null,
  status: 'WATCHING',
  progress: null,
  episodesCount: null,
  seasonYear: null,
  season: null,
  sources: [],
  nextEpisode: null,
  broadcasts: [],
  recentBroadcasts: [],
  recordedEpisodes: [],
};

// worksAt は、now を今として、作品を返す。放送の時期は、番組表と同じ放送日 (4 時から翌 4 時) で決まるので、日の区切りから決める。
const worksAt = (now: number) => {
  const today = broadcastDay(now);
  const yesterday = broadcastDay(today.from - 1);

  return [
    // 今日 (放送中)。作品の画像、Annict と AniList、見終えた話数と全話数、予約済み、ルールの対象 (予約済み)。
    // 録画のある話は、6 話を超えるので、残りを数で示す。2 回録った話と、2 話をまとめた録画もある。
    {
      ...base,
      id: 'AnimeWork:1',
      tid: 1001,
      title: '星の旅人',
      imageUrl: image('#e8743b'),
      progress: 2,
      episodesCount: 12,
      seasonYear: 2026,
      season: 'AUTUMN',
      sources: [
        {
          destination: 'ANNICT',
          workId: '1',
          status: 'WATCHING',
          url: 'https://anime.example.com/works/1',
        },
        {
          destination: 'ANILIST',
          workId: '1',
          status: 'WANNA_WATCH',
          url: 'https://list.example.com/anime/1',
        },
      ],
      nextEpisode: { number: 3, numberText: null, title: '嵐の夜' },
      broadcasts: [broadcast('星の旅人', 3, now - 10 * 60_000, { reservation: { enabled: true } })],
      recordedEpisodes: [
        {
          countStart: 1,
          countEnd: 2,
          subtitle: null,
          recordingCount: 1,
          watched: false,
          recording: { id: 'Recording:anime-1', thumbnailUrl: null },
        },
        ...recorded(3, 3, { recordingCount: 2 }),
        ...recorded(4, 9),
      ],
    },
    // 明日。画像がないので、録画の代表画像を出す。年だけ分かる。全話数だけ分かる。次の話は番号の代わりの文。
    // 次の放送は再放送で、予約を無効にしている。見たかが分からない録画 (見たかを示さない)。ルールを作れる。
    {
      ...base,
      id: 'AnimeWork:2',
      tid: 1002,
      title: '海辺の喫茶店',
      episodesCount: 10,
      seasonYear: 2026,
      nextEpisode: { number: null, numberText: '特別編', title: null },
      broadcasts: [
        broadcast('海辺の喫茶店', 4, today.to + 2 * hour, {
          rerun: true,
          reservation: { enabled: false },
        }),
      ],
      recordedEpisodes: recorded(1, 2, { watched: null, thumbnailUrl: image('#19a979') }),
    },
    // それより後 (バッジなし)。画像も代表画像もない。見終えた話数だけ分かる。次の話が分からない。予約していない。
    {
      ...base,
      id: 'AnimeWork:3',
      tid: null,
      title: '時計塔の少女',
      progress: 5,
      broadcasts: [broadcast('時計塔の少女', 6, today.to + 30 * hour)],
    },
    // まもなく始まる (分で示す)。予約していない。
    {
      ...base,
      id: 'AnimeWork:4',
      tid: 1004,
      title: '朝の体操クラブ',
      nextEpisode: { number: 8, numberText: null, title: null },
      broadcasts: [broadcast('朝の体操クラブ', 8, now + 20 * 60_000)],
    },
    // 昨日。これからの放送がない。見たいリストの作品。
    {
      ...base,
      id: 'AnimeWork:5',
      tid: null,
      title: '風のうた',
      status: 'WANNA_WATCH',
      recentBroadcasts: [{ startAt: at(yesterday.from + hour), rerun: false, last: false }],
    },
    // 終了 (最も新しい放送が最終話)。作品の画像が読めないので、枠を消す。
    {
      ...base,
      id: 'AnimeWork:6',
      tid: null,
      title: '雪の街',
      imageUrl: `${import.meta.env.BASE_URL}missing-anime-image.png`,
      recentBroadcasts: [{ startAt: at(yesterday.from - 48 * hour), rerun: false, last: true }],
    },
    // 未定 (放送が分からない)。見たいリストの作品。
    { ...base, id: 'AnimeWork:7', tid: null, title: '未来図鑑', status: 'WANNA_WATCH' },
  ];
};

const annictImported = () => ({
  destination: 'ANNICT',
  importedAt: at(Date.now() - hour),
  error: null,
  entryCount: 7,
});

const library = {
  animeLibrary: () => ({
    enabled: true,
    // まだ取り込んでいない記録先 (AniList) もある。
    imports: [
      annictImported(),
      { destination: 'ANILIST', importedAt: null, error: null, entryCount: 0 },
    ],
    works: worksAt(Date.now()),
  }),
};

const wishlist = {
  animeWishlist: () => ({
    enabled: true,
    imports: [],
    works: [
      { tid: 1001, covered: true },
      { tid: 1002, covered: false },
      { tid: 1004, covered: false },
    ],
  }),
};

const filled = { ...library, ...wishlist };

// signedOut は、作品の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'AnimeLibraryQuery') {
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

// 既定。管理者がアニメの連携を有効にしていないとき。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { animeLibrary: failing() } } };

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// 連携は有効だが、ユーザーが Annict とも AniList とも連携していないとき (連携の設定へのリンク)。
export const NotConnected: Story = {
  args: { overrides: { animeLibrary: () => ({ enabled: true, imports: [], works: [] }) } },
};

// 見てる作品 (既定の絞り込み)。取り込んだ記録先と、まだ取り込んでいない記録先を示す。
export const Watching: Story = { args: { overrides: filled } };

// すべての作品。放送の時期 (今日、昨日、明日、終了、それより後、未定) のそれぞれの作品を並べる。
export const All: Story = { args: { url: '/anime?status=all', overrides: filled } };

// 再放送を除く絞り込み。再放送しか次の放送がない作品は、次の放送がないものとして出す。
export const ExcludeReruns: Story = {
  args: { url: '/anime?status=all&rerun=exclude', overrides: filled },
};

// 絞り込みに当たる作品がないとき。
export const NoMatch: Story = {
  args: { url: '/anime?season=2020-WINTER', overrides: filled },
};

// 取り込みに失敗した記録先があるとき。
export const ImportFailed: Story = {
  args: {
    overrides: {
      ...wishlist,
      animeLibrary: () => ({
        enabled: true,
        imports: [
          annictImported(),
          {
            destination: 'ANILIST',
            importedAt: null,
            error: 'the token has expired',
            entryCount: 0,
          },
        ],
        works: worksAt(Date.now()),
      }),
    },
  },
};

// 配信サービスを 1 つも選んでいないとき (配信で探すメニューを出さない)。
export const NoStreaming: Story = {
  args: { overrides: filled },
  beforeEach: () => withStorage({ 'kotatsu.anime.streamingServices': '[]' }),
};

// 視聴者 (予約やルールを変えられない) のとき。予約とルールのボタンを出さない。記録は頼める。
export const AsViewer: Story = {
  args: { overrides: merge(() => filled, asViewer('VIEWER')) },
};

// ルールを作ったとき (作ったルールへのリンク)。
export const RuleCreated: Story = {
  args: {
    overrides: {
      ...filled,
      createAnimeWishlistRule: () => ({ id: 'Rule:anime', name: '海辺の喫茶店' }),
    },
  },
  play: click('ルールを作る'),
};

// ルールを作れなかったとき。
export const RuleFailed: Story = {
  args: { overrides: { ...filled, createAnimeWishlistRule: failing() } },
  play: click('ルールを作る'),
};

// 予約できなかったとき。
export const ReserveFailed: Story = {
  args: { overrides: { ...filled, createProgramReservation: failing() } },
  play: click('予約する'),
};

// 見たことの記録を頼んだとき。
export const RecordRequested: Story = {
  args: { overrides: { ...filled, recordAnimeEpisodes: () => true } },
  play: click('第3話を見たことを記録する'),
};

// 見たことを記録済みだったとき。
export const RecordAlready: Story = {
  args: { overrides: { ...filled, recordAnimeEpisodes: failingWith({ code: 'CONFLICT' }) } },
  play: click('第3話を見たことを記録する'),
};

// 見たことの記録を頼めなかったとき。
export const RecordFailed: Story = {
  args: { overrides: { ...filled, recordAnimeEpisodes: failing() } },
  play: click('第3話を見たことを記録する'),
};
