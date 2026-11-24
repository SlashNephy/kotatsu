import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { demoStreamURL } from '../../demo/epg';
import { call, liveStreamScrambled, merge, nodeWith, withStorage } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// ライブ視聴 (/live/$serviceId) の画面 (#2056)。静的なデモと同じ Service Worker で、デモの映像を繰り返して流す。
// 視聴が止まったとき (つなぎ直し、録画にチューナーを取られた、スクランブル)、番組がないとき、タイムシフト、再生の経路を並べる。
const service = 'Service:1024';

const meta = {
  title: 'ページ/ライブ/視聴',
  component: PageStory,
  args: { url: `/live/${service}` },
  parameters: { layout: 'fullscreen' },
  // 音を出さない。
  beforeEach: () =>
    withStorage({ 'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }) }),
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 映像が届かず、視聴が止まって、つなぎ直すまで数えているとき。
export const Disconnected: Story = {
  args: { overrides: nodeWith(service, { streamUrl: '/demo/missing.m2ts' }) },
};

// 録画にチューナーを取られて、視聴が止まったとき。止まった頃に始まった録画を、理由として出す。
export const DisconnectedByRecording: Story = {
  args: {
    overrides: merge(nodeWith(service, { streamUrl: '/demo/missing.m2ts' }), (root: DemoRoot) => ({
      recordings: (args: { filter?: { states?: string[] } | null }) => {
        if (args.filter?.states?.includes('RECORDING') !== true) {
          return call<object>(root, 'recordings', args);
        }
        const node = {
          id: 'Recording:recording-now',
          name: 'ねこの町を歩く',
          recordedFrom: new Date().toISOString(),
          service: { id: 'Service:1024', name: 'こたつテレビ', logoUrl: null },
        };

        return {
          edges: [{ node, cursor: '0' }],
          pageInfo: {
            hasNextPage: false,
            endCursor: '0',
            hasPreviousPage: false,
            startCursor: '0',
          },
        };
      },
    })),
  },
};

// スクランブルされたまま届くと決まり、つなぎ直すのをやめたとき (B-CAS カードが読めていないかもしれない)。
export const Scrambled: Story = { args: { overrides: liveStreamScrambled(service) } };

// 今の時刻の番組が番組表にないとき。
export const NoProgram: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      guide: (args: object) =>
        call<object[]>(root, 'guide', args).map((row) => ({ ...row, programs: [] })),
    }),
  },
};

// openMoreActions は、番組名の横の「そのほかの操作」のメニューを開く。番組の詳細、外部プレイヤー、タイムシフトの項目の出し分けを見る。
const openMoreActions: Story['play'] = async ({ canvas }) => {
  await userEvent.click(await canvas.findByRole('button', { name: 'そのほかの操作' }));
  await within(document.body).findByRole('menu');
};

// そのほかの操作のメニュー (番組の詳細と外部プレイヤーの項目)。
export const MoreActions: Story = { play: openMoreActions };

// 番組がないときのメニュー (番組の詳細の項目を出さない)。
export const MoreActionsWithoutProgram: Story = {
  args: { overrides: NoProgram.args?.overrides },
  play: openMoreActions,
};

// 管理者が、外部プレイヤー用の URL を無効にしているとき。
export const ExternalPlayerUrlsDisabled: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      systemSettings: () => ({
        ...call<object>(root, 'systemSettings'),
        externalPlayerUrlsEnabled: false,
      }),
    }),
  },
  play: openMoreActions,
};

// タイムシフトのバッファがあり、さかのぼって見られるとき。
const timeshift = nodeWith(service, {
  timeshiftPrograms: [{ startAt: new Date(Date.now() - 30 * 60_000).toISOString() }],
  timeshiftBuffer: {
    from: new Date(Date.now() - 30 * 60_000).toISOString(),
    gaps: [],
    streamUrl: demoStreamURL,
    hlsUrl: '/demo/timeshift/playlist.m3u8',
  },
});

export const Timeshift: Story = { args: { overrides: timeshift } };

// タイムシフトの番組の一覧を開く項目を出したメニュー。
export const TimeshiftMoreActions: Story = {
  args: { overrides: timeshift },
  play: openMoreActions,
};

// タイムシフトで、ライブの端より後ろを見ているとき (一時停止してさかのぼった)。
export const TimeshiftBehind: Story = {
  args: { overrides: timeshift },
  play: async ({ canvas }) => {
    // 一時停止のボタンは、映像を読み込んでから出るので、長めに待つ。
    await userEvent.click(
      await canvas.findByRole('button', { name: '一時停止' }, { timeout: 20_000 }),
    );
  },
};

// データ放送を出しているとき。
export const DataBroadcast: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: /データ放送/ }));
  },
};

// 再生の経路ごとの表示。kyusu がいるときは、変換した HLS を流す (Storybook には kyusu がないので、映像は届かない)。
export const KyusuRoute: Story = {
  args: {
    overrides: {
      transcoding: () => ({
        available: true,
        presets: [
          { name: '720p', codec: 'H264', passthrough: false, videoBitrate: 3_000_000 },
          { name: '1080p', codec: 'H264', passthrough: false, videoBitrate: 6_000_000 },
        ],
      }),
    },
  },
  beforeEach: () =>
    withStorage({
      'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }),
      'kotatsu.player.quality': '720p',
    }),
};

// スーパーハイビジョン (8K) のサービスは、再生できない。
export const UnavailableSHV: Story = {
  args: { overrides: nodeWith(service, { videoFormat: 'SHV' }) },
};

// BS4K のサービスで、kyusu がいないとき (ブラウザの中では変換できない)。
export const UnavailableUHDNoTranscoder: Story = {
  args: { overrides: nodeWith(service, { videoFormat: 'UHD' }) },
};
