import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { asViewer, call, merge, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 字幕の外字 (/admin/captions) の画面 (#2056)。置き換えのテーブルの有無、入力の誤り、足している途中、保存の失敗を並べる。
// 字幕の自動生成のモデル (CaptionModels) は /admin/recording の画面にあるので、そちらの Story で見る。
const meta = {
  title: 'ページ/管理/字幕',
  component: PageStory,
  args: { url: '/admin/captions' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// withRows は、外字の置き換えのテーブルを rows にする。ほかの全体の設定は、デモのものを使う。
const withRows = (rows: { md5: string; text: string }[]) => (root: DemoRoot) => ({
  systemSettings: () => ({
    ...call<object>(root, 'systemSettings'),
    drcsReplacements: rows,
    updatedAt: minutesAgo(90),
  }),
});

const rows = [
  { md5: '0123456789abcdef0123456789abcdef', text: '①' },
  { md5: 'fedcba9876543210fedcba9876543210', text: '[字]' },
];

// まだ登録していないとき (デモの既定)。最終更新もない。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 外字を登録しているとき。
export const Rows: Story = { args: { overrides: withRows(rows) } };

// 登録した外字の置き換える文字を空にして、欄を離れたとき (文字の数の誤り)。
export const InvalidText: Story = {
  args: { overrides: withRows(rows) },
  play: async ({ canvas }) => {
    const [first] = await canvas.findAllByRole('textbox', { name: '置き換える文字' });
    if (first !== undefined) {
      await userEvent.clear(first);
      await userEvent.tab();
    }
  },
};

// 足せる外字を入れたとき (足すのボタンを押せる)。
export const NewValid: Story = {
  args: { overrides: withRows(rows) },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('MD5'), '00112233445566778899aabbccddeeff');
    const inputs = await canvas.findAllByRole('textbox', { name: '置き換える文字' });
    const added = inputs.at(-1);
    if (added !== undefined) {
      await userEvent.type(added, '㊙');
    }
  },
};

// 外字を足している途中 (足すのボタンを押せない)。
export const Adding: Story = {
  args: { overrides: withRows(rows), hold: ({ name }) => name === 'captionsMutation' },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('MD5'), '00112233445566778899aabbccddeeff');
    const inputs = await canvas.findAllByRole('textbox', { name: '置き換える文字' });
    const added = inputs.at(-1);
    if (added !== undefined) {
      await userEvent.type(added, '㊙');
    }
    await userEvent.click(await canvas.findByRole('button', { name: '足す' }));
  },
};

// 外字を消せなかったとき。
export const SaveFailed: Story = {
  args: {
    overrides: (root: DemoRoot) => ({ ...withRows(rows)(root), updateSystemSettings: failing() }),
  },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '消す' });
    if (remove !== undefined) {
      await userEvent.click(remove);
    }
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で拒否する)。全体の設定はナビゲーションも読むので、外字のテーブルだけを拒否する。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), (root: DemoRoot) => ({
      systemSettings: () => ({
        ...call<object>(root, 'systemSettings'),
        drcsReplacements: failingWith({ code: 'FORBIDDEN' }),
      }),
    })),
  },
};
