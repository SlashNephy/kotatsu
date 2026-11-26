import type { Meta, StoryObj } from '@storybook/react-vite';

import { demoStreamURL } from '../../demo/streamURL';
import { GraphQLRequestError } from '../../relay/requestError';
import { minutesAgo, withStorage } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// タイムシフトの再生 (/timeshift/$serviceId/play) の画面 (#2056)。静的なデモと同じ Service Worker で、デモの映像を流す。
// 終わった番組と放送中の番組、途切れた区間、名前のない番組、URL の位置からの再生、番組がないとき、
// ログインしていないとき、読み込めないとき、映像を読めないとき、再生の経路を並べる。
const startAt = minutesAgo(90);

const meta = {
  title: 'ページ/タイムシフト/再生',
  component: PageStory,
  args: { url: `/timeshift/Service:1024/play?program=${encodeURIComponent(startAt)}` },
  parameters: { layout: 'fullscreen' },
  // 音を出さない。
  beforeEach: () =>
    withStorage({ 'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }) }),
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// program は、バッファにある 30 分の番組。
const program = (patch: Record<string, unknown> = {}) => ({
  timeshiftProgram: () => ({
    name: '[字]ドキュメンタリー「森の一年」',
    startAt,
    endAt: minutesAgo(60),
    playableFrom: startAt,
    gaps: [],
    streamUrl: demoStreamURL,
    hlsUrl: '/demo/timeshift/playlist.m3u8',
    service: { id: 'Service:1024', name: 'こたつテレビ', logoUrl: null, videoFormat: 'HD' },
    ...patch,
  }),
});

export const Default: Story = { args: { overrides: program() } };

export const Loading: Story = { args: { hold: holdAll } };

// 放送中の番組 (終わりの時刻がまだない)。
export const OnAir: Story = { args: { overrides: program({ endAt: null }) } };

// 録れていない区間 (チューナーを取られたなど) があるとき。シークバーにマークを出す。
export const WithGaps: Story = {
  args: {
    overrides: program({ gaps: [{ from: minutesAgo(80), to: minutesAgo(75) }] }),
  },
};

// 番組の先頭が録れておらず、途中から再生できるとき。
export const PlayableFromMiddle: Story = {
  args: { overrides: program({ playableFrom: minutesAgo(85) }) },
};

// 名前のない番組。
export const Untitled: Story = { args: { overrides: program({ name: '' }) } };

// URL の位置 (2 分) から再生するとき。
export const StartAt: Story = {
  args: {
    overrides: program(),
    url: `/timeshift/Service:1024/play?program=${encodeURIComponent(startAt)}&t=120`,
  },
};

// バッファから消えた番組。
export const NotFound: Story = {
  args: { overrides: { timeshiftProgram: failingWith({ code: 'NOT_FOUND' }) } },
};

// ログインしていないとき (共有された URL を開いた)。
export const SignedOut: Story = {
  args: {
    hold: (operation: { name: string }) => {
      if (operation.name === 'ServiceIdPlayTimeshiftQuery') {
        throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
      }

      return false;
    },
  },
};

export const Error: Story = { args: { overrides: { timeshiftProgram: failing() } } };

// 映像を読めないとき。
export const PlaybackError: Story = {
  args: { overrides: program({ streamUrl: '/demo/missing.m2ts' }) },
};

// kyusu がいて、変換した HLS で再生するとき (Storybook には kyusu がないので、映像は届かない)。
export const KyusuRoute: Story = {
  args: {
    overrides: {
      ...program(),
      transcoding: () => ({
        available: true,
        presets: [{ name: '720p', codec: 'H264', passthrough: false, videoBitrate: 3_000_000 }],
      }),
    },
  },
  beforeEach: () =>
    withStorage({
      'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }),
      'kotatsu.player.quality': '720p',
    }),
};

// BS4K のサービスで、kyusu がいないとき (ブラウザの中では変換できない)。
export const UnavailableUHD: Story = {
  args: {
    overrides: program({
      service: { id: 'Service:1025', name: 'こたつ4K', logoUrl: null, videoFormat: 'UHD' },
    }),
  },
};
