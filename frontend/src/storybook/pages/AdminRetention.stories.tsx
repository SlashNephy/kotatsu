import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 保存期間 (/admin/retention) の画面 (#2056)。
const meta = {
  title: 'ページ/管理/保存期間',
  component: PageStory,
  args: { url: '/admin/retention' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// retention は、どの記録も日数を決めてあり、最終更新の時刻のある状態。
const retention = (root: DemoRoot) => ({
  systemSettings: () => ({
    ...call<object>(root, 'systemSettings'),
    watchProgressRetentionDays: 365,
    networkActivityRetentionDays: 14,
    auditRetentionDays: 365,
    auditClientInfoRetentionDays: 90,
    updatedAt: '2026-11-20T12:00:00+09:00',
  }),
});

// 日数を変えたことがなく、最終更新の時刻がないとき (デモの既定)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 日数を変えたことがあり、最終更新の時刻を出すとき。
export const Customized: Story = { args: { overrides: retention } };

// failingLeaf は、この画面だけが読む日数の欄を fail で失敗させる。ヘッダーも読む全体の設定は、失敗させない。
const failingLeaf = (fail: () => never) => (root: DemoRoot) => ({
  systemSettings: () => ({ ...call<object>(root, 'systemSettings'), archiveRetentionDays: fail }),
});

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: failingLeaf(failingWith({ code: 'FORBIDDEN' })) },
};

// 設定を読めなかったとき。
export const Failed: Story = { args: { overrides: failingLeaf(failing()) } };

// 下限より小さい日数を打って欄を離れ、入力の誤りを出したとき (通信の記録は 1 日以上)。
export const OutOfRange: Story = {
  args: { overrides: retention },
  play: async ({ canvas }) => {
    // どの欄も同じ名前なので、並びで選ぶ。4 つ目が通信の記録の欄。
    const field = (await canvas.findAllByRole('textbox', { name: '保持期間（日）' }))[3];
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.clear(field);
    await userEvent.type(field, '0');
    await userEvent.tab();
  },
};

// 日数を保存できなかったとき (失敗の通知を出す)。
export const SaveFailed: Story = {
  args: { overrides: { updateSystemSettings: failing() } },
  play: async ({ canvas }) => {
    const [field] = await canvas.findAllByRole('textbox', { name: '保持期間（日）' });
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.clear(field);
    await userEvent.type(field, '7');
    await userEvent.tab();
  },
};
