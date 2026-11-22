import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import { merge, minutesAgo, recordingsWith, withStorage } from '../conditions';
import { firstRecordingPath, holdAll, PageStory } from '../PageStory';

// 録画の再生 (/recordings/$recordingId/play) の画面 (#2056)。静的なデモと同じ Service Worker で、デモの映像を流す。
// 続きからの再生、CM の区間 (飛ばすボタンと自動で飛ばしたとき)、自動生成の字幕、映像を読めないとき、再生の経路を並べる。
// いちばん新しい録画を、放送を終えた録画にして開く。
const meta = {
  title: 'ページ/録画/再生',
  component: PageStory,
  args: { url: firstRecordingPath('/recordings/:id/play') },
  parameters: { layout: 'fullscreen' },
  // 音を出さない。
  beforeEach: () =>
    withStorage({ 'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }) }),
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// ended は、30 分の放送を録り終えた録画。
const ended = {
  state: 'COMPLETED',
  startAt: minutesAgo(90),
  endAt: minutesAgo(60),
  recordedFrom: minutesAgo(90),
  recordedUntil: minutesAgo(60),
  liveStats: null,
  watchPositionMs: null,
};

export const Default: Story = { args: { overrides: recordingsWith(ended) } };

export const Loading: Story = { args: { hold: holdAll } };

// 前回の続き (5 分) から再生するとき。
export const Resumed: Story = {
  args: { overrides: recordingsWith({ ...ended, watchPositionMs: 5 * 60_000 }) },
};

// URL の位置 (2 分) から再生するとき。
export const StartAt: Story = {
  args: {
    overrides: recordingsWith(ended),
    url: firstRecordingPath('/recordings/:id/play?t=120'),
  },
};

// 再生の始まりが CM の区間のとき。CM を飛ばすボタンを出す。
const inCm = recordingsWith({
  ...ended,
  cmSections: [{ from: minutesAgo(90), to: minutesAgo(89) }],
});

export const InCm: Story = { args: { overrides: inCm } };

// CM を自動で飛ばす設定で、始まりの CM を飛ばして再生するとき。
export const InCmAutoSkip: Story = {
  args: { overrides: inCm },
  beforeEach: () =>
    withStorage({
      'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }),
      'kotatsu.player.cmAutoSkip': 'true',
    }),
};

// 自動生成の字幕を選んで出しているとき。
export const AutoCaptions: Story = {
  args: {
    overrides: recordingsWith({
      ...ended,
      autoCaptions: [
        {
          from: minutesAgo(90),
          to: minutesAgo(89.9),
          text: 'こたつ山のふもとから、けさの様子をお伝えします',
        },
        {
          from: minutesAgo(89.9),
          to: minutesAgo(89.8),
          text: '空気は冷たく、畑には霜が降りています',
        },
      ],
    }),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '設定' }, { timeout: 20_000 }));
    const menu = within(document.body);
    await userEvent.click(await menu.findByRole('combobox', { name: '字幕の種類' }));
    await userEvent.click(await menu.findByRole('option', { name: '自動生成' }));
  },
};

// 録画の映像を読めないとき。
export const Error: Story = {
  args: { overrides: recordingsWith({ ...ended, streamUrl: '/demo/missing.m2ts' }) },
};

// もとのルールがある録画 (ルールへのリンクを出す) と、ないもの。
export const WithoutRule: Story = {
  args: { overrides: recordingsWith({ ...ended, rule: null, ruleName: null }) },
};

// kyusu がいて、変換した HLS で再生するとき (Storybook には kyusu がないので、映像は届かない)。
export const KyusuRoute: Story = {
  args: {
    overrides: merge(recordingsWith(ended), () => ({
      transcoding: () => ({
        available: true,
        presets: [{ name: '720p', codec: 'H264', passthrough: false, videoBitrate: 3_000_000 }],
      }),
    })),
  },
  beforeEach: () =>
    withStorage({
      'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }),
      'kotatsu.player.quality': '720p',
    }),
};

// BS4K の録画で、kyusu がいないとき (ブラウザの中では変換できない)。
export const UnavailableUHD: Story = {
  args: { overrides: recordingsWith({ ...ended, videoFormat: 'UHD' }) },
};
