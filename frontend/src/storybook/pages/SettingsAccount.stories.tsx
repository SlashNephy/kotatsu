import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// アカウントの連携の設定 (/settings/account) の画面 (#2056)。アニメの記録先 (Annict、AniList) と、ドラマの記録先 (Simkl) の
// 連携の有無、管理者が無効にしているとき、記録先から戻ってきたときの結果を並べる。
const meta = {
  title: 'ページ/設定/アカウント',
  component: PageStory,
  args: { url: '/settings/account' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const connectedAt = '2026-11-01T12:00:00+09:00';

const animeSettings = (connection: (destination: string) => object | null, available = true) => ({
  enabled: true,
  thresholdPercent: 20,
  watchingOnFirst: true,
  watchingOnEpisode: false,
  watchedOnLast: true,
  notifyWishlist: true,
  destinations: ['ANNICT', 'ANILIST'].map((destination) => ({
    destination,
    available,
    connection: connection(destination),
  })),
});

const dramaSettings = (connection: object | null, available = true) => ({
  enabled: true,
  thresholdPercent: 20,
  destinations: [{ destination: 'SIMKL', available, connection }],
});

const simkl = {
  accountName: 'kotatsu',
  connectedAt,
  reauthorizationRequired: false,
  watchlistImportedAt: '2026-11-20T08:00:00+09:00',
  watchlistError: null,
};

// 管理者が、アニメとドラマの記録を無効にしているとき (デモの既定)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 記録先を登録しているが、まだ連携していないとき。
export const NotConnected: Story = {
  args: {
    overrides: {
      animeRecordSettings: () => animeSettings(() => null),
      dramaRecordSettings: () => dramaSettings(null),
    },
  },
};

// 管理者が記録先を登録していないとき (連携の機能は有効)。
export const NotRegistered: Story = {
  args: {
    overrides: {
      animeRecordSettings: () => animeSettings(() => null, false),
      dramaRecordSettings: () => dramaSettings(null, false),
    },
  },
};

// どの記録先とも連携しているとき。AniList には、AniList の画面からも外す案内を出す。
export const Connected: Story = {
  args: {
    overrides: {
      animeRecordSettings: () =>
        animeSettings((destination) => ({ accountName: destination.toLowerCase(), connectedAt })),
      dramaRecordSettings: () => dramaSettings(simkl),
    },
  },
};

// Simkl の連携のやり直しが要るとき。
export const SimklReauthorizationRequired: Story = {
  args: {
    overrides: {
      animeRecordSettings: () => animeSettings(() => null),
      dramaRecordSettings: () => dramaSettings({ ...simkl, reauthorizationRequired: true }),
    },
  },
};

// Simkl の見たいリストを取り込めなかったとき。
export const SimklWatchlistError: Story = {
  args: {
    overrides: {
      animeRecordSettings: () => animeSettings(() => null),
      dramaRecordSettings: () =>
        dramaSettings({ ...simkl, watchlistImportedAt: null, watchlistError: 'rate limited' }),
    },
  },
};

// 記録先と連携して戻ってきたとき。
export const ConnectedCallback: Story = {
  args: {
    url: '/settings/account?connected=annict',
    overrides: NotConnected.args?.overrides,
  },
};

export const SimklConnectedCallback: Story = {
  args: {
    url: '/settings/account?connected=simkl',
    overrides: Connected.args?.overrides,
  },
};

// 記録先から、連携に失敗して戻ってきたとき (理由ごと)。
const callbackError = (error: string): Story => ({
  args: { url: `/settings/account?error=${error}`, overrides: NotConnected.args?.overrides },
});

export const CallbackExpired = callbackError('anime_record_expired');
export const CallbackMismatch = callbackError('anime_record_mismatch');
export const CallbackDenied = callbackError('anime_record_denied');
export const CallbackUnavailable = callbackError('anime_record_unavailable');
export const CallbackFailed = callbackError('anime_record_failed');
export const SimklCallbackDenied = callbackError('drama_record_denied');
