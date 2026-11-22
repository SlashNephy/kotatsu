import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { failing, holdAll, PageStory } from '../PageStory';

// リモコン (/remote) の画面 (#2056)。操作できるプレイヤーがないとき、ライブ (データ放送を出しているかどうか) と録画の再生
// (位置が分かるかどうか)、複数のプレイヤー、音声の切り替え、別の録画を開かせる欄、操作に失敗したときの表示を並べる。
const meta = {
  title: 'ページ/リモコン',
  component: PageStory,
  args: { url: '/remote' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const desktop =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36';
const tv = 'ExampleTV/1.0';

const liveState = {
  kind: 'LIVE',
  title: '[字]夜のニュース',
  paused: false,
  position: null,
  duration: null,
  volume: 1,
  muted: false,
  audios: [],
  audio: null,
  dataBroadcast: false,
  keyGroups: [],
};

const recordingState = {
  kind: 'RECORDING',
  title: 'ドキュメンタリー「森の一年」',
  paused: true,
  position: 754,
  duration: 3_540,
  volume: 0.6,
  muted: true,
  audios: [
    { id: 'main', label: '主音声' },
    { id: 'sub', label: '副音声' },
  ],
  audio: 'sub',
  dataBroadcast: false,
  keyGroups: [],
};

const player = (id: string, userAgent: string, state: object, current = false) => ({
  id,
  current,
  userAgent,
  lastSeenAt: new Date().toISOString(),
  state,
});

// 既定。同じアカウントのほかの端末で、プレイヤーを開いていないとき。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { players: failing() } } };

// 開いているのが自分の端末のプレイヤーだけのとき (操作する相手にしない)。
export const OnlyCurrent: Story = {
  args: { overrides: { players: () => [player('Player:self', desktop, liveState, true)] } },
};

// ほかの端末でライブを見ているとき。データ放送を出していないので、出すボタンを置く。
export const Live: Story = {
  args: { overrides: { players: () => [player('Player:1', desktop, liveState)] } },
};

// ライブでデータ放送を出しているとき (データ放送のリモコン)。番組名が分からない。
export const LiveDataBroadcast: Story = {
  args: {
    overrides: {
      players: () => [
        player('Player:1', desktop, {
          ...liveState,
          title: null,
          dataBroadcast: true,
          keyGroups: ['basic', 'data-button', 'numeric-tuning'],
        }),
      ],
    },
  },
};

// データ放送が、決定と矢印のボタンしか受け付けないとき (色と数字のボタンは押せない)。数字のボタンを開く。
export const LiveDataBroadcastBasicKeys: Story = {
  args: {
    overrides: {
      players: () => [
        player('Player:1', desktop, { ...liveState, dataBroadcast: true, keyGroups: ['basic'] }),
      ],
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '数字' }));
  },
};

// 録画を止めて見ているとき。位置のスライダー、早送りと巻き戻し、消音、音声の切り替え。
export const Recording: Story = {
  args: { overrides: { players: () => [player('Player:1', desktop, recordingState)] } },
};

// 録画の位置が分からないとき (読み込みの途中)。スライダーと早送りを出さない。
export const RecordingWithoutPosition: Story = {
  args: {
    overrides: {
      players: () => [
        player('Player:1', desktop, {
          ...recordingState,
          position: null,
          paused: false,
          audios: [],
        }),
      ],
    },
  },
};

// 操作できるプレイヤーが複数あるとき。端末の名前で選ぶ。名前の分からない端末もある。
export const Players: Story = {
  args: {
    overrides: {
      players: () => [
        player('Player:1', desktop, liveState),
        player('Player:2', tv, recordingState),
      ],
    },
  },
};

// 操作を送れなかったとき。
export const CommandFailed: Story = {
  args: {
    overrides: {
      players: () => [player('Player:1', desktop, liveState)],
      sendPlayerCommand: failing(),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '一時停止' }));
  },
};

// 開かせる録画を探したとき。
export const OpenRecording: Story = {
  args: { overrides: { players: () => [player('Player:1', desktop, liveState)] } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('radio', { name: '録画' }));
    await userEvent.click(await canvas.findByRole('button', { name: '探す' }));
  },
};

// 開かせる録画を探して、当たらなかったとき。
export const OpenRecordingNoMatch: Story = {
  args: {
    overrides: {
      players: () => [player('Player:1', desktop, liveState)],
      recordings: () => ({ edges: [], pageInfo: { hasNextPage: false, endCursor: null } }),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('radio', { name: '録画' }));
    await userEvent.type(
      await canvas.findByRole('textbox', { name: '番組名で録画を探す' }),
      '該当なし',
    );
    await userEvent.click(await canvas.findByRole('button', { name: '探す' }));
  },
};

// 開かせる録画を探せなかったとき。
export const OpenRecordingFailed: Story = {
  args: {
    overrides: { players: () => [player('Player:1', desktop, liveState)], recordings: failing() },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('radio', { name: '録画' }));
    await userEvent.click(await canvas.findByRole('button', { name: '探す' }));
  },
};
