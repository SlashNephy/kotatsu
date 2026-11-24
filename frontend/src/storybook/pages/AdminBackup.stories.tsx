import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { asViewer, merge, minutesAgo } from '../conditions';
import { failingWith, holdAll, PageStory } from '../PageStory';

// バックアップ (/admin/backup) の画面 (#2056)。最後の結果 (まだない、バックアップした、失敗した)、公開鍵の有無と入力、
// 今すぐバックアップするよう頼んだとき、保存の途中と失敗を並べる。
const meta = {
  title: 'ページ/管理/バックアップ',
  component: PageStory,
  args: { url: '/admin/backup' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// 架空の公開鍵。画面は形 (age1 と Bech32 の文字) だけを確認する。
const keys = [
  'age1kq7tz9xyadem3d5ua8zm4fhgxl0cevsjn2p6wrqt9s5h3yuk7ancxdg2zq',
  'age1x8d2m0sw7pfajcne3tzgq5hy9ku6lv4r2xcm8nqd0ts7ey3wazjh5fgp9u',
];
const newKey = 'age1zw5qn8c3d7ajh0kxmseu2tg9fy6vlp4r3dq8wcen5mx7sj0tak2hzgyf6u';

const settings = { recipients: keys, intervalHours: 24, generations: 7, updatedAt: minutesAgo(60) };

const withRun = (latestRun: object | null) => ({
  backup: () => ({ settings, latestRun }),
});

const succeeded = {
  manual: false,
  startedAt: minutesAgo(65),
  finishedAt: minutesAgo(62),
  fileName: 'kotatsu-20261122-0300.tar.age',
  size: 1_288_490_188,
  error: null,
};

// 公開鍵を登録しておらず、まだバックアップしていないとき (デモの既定)。「今すぐバックアップする」ボタンは押せない。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 定期のバックアップを書き出せたとき (ファイルの名前と大きさを出す)。公開鍵を 2 つ登録している。
export const Succeeded: Story = { args: { overrides: withRun(succeeded) } };

// 手動のバックアップに失敗したとき (ファイルはなく、エラーを出す)。
export const RunFailed: Story = {
  args: {
    overrides: withRun({
      ...succeeded,
      manual: true,
      fileName: null,
      size: null,
      error: 'open /backup/kotatsu-20261122-0300.tar.age: permission denied',
    }),
  },
};

// 今すぐバックアップするよう頼み、結果を待っているとき。
export const RunRequested: Story = {
  args: {
    overrides: {
      ...withRun(succeeded),
      runBackup: () => ({ settings: { updatedAt: minutesAgo(0) } }),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '今すぐ書き出す' }));
  },
};

// 公開鍵の形が違うものを入れたとき。
export const InvalidKey: Story = {
  args: { overrides: withRun(succeeded) },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('公開鍵を足す'), 'ssh-ed25519');
  },
};

// 足せる公開鍵を入れたとき (足すのボタンを押せる)。
export const ValidKey: Story = {
  args: { overrides: withRun(succeeded) },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('公開鍵を足す'), newKey);
  },
};

// 公開鍵の一覧を保存している途中。読み直すまで、足すのと消すのを止める。
export const SavingKeys: Story = {
  args: {
    overrides: withRun(succeeded),
    hold: ({ name }) => name === 'backupMutation',
  },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '消す' });
    if (remove !== undefined) {
      await userEvent.click(remove);
    }
  },
};

// 設定を保存できなかったとき (デモはバックアップの設定を変えさせない)。
export const SaveFailed: Story = {
  args: { overrides: withRun(succeeded) },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '消す' });
    if (remove !== undefined) {
      await userEvent.click(remove);
    }
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で断る)。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), () => ({ backup: failingWith({ code: 'FORBIDDEN' }) })),
  },
};
