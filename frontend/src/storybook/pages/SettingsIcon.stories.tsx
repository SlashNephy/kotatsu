import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { withStorage } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// アイコンの設定 (/settings/icon) の画面 (#2056)。既定のアイコン、この端末で選んだアイコン、選んで保存を通知したときを並べる。
const meta = {
  title: 'ページ/設定/アイコン',
  component: PageStory,
  args: { url: '/settings/icon' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// iconKey は、選んだアイコンを保存する localStorage のキー (src/settings/appIcon.ts)。
const iconKey = 'kotatsu.appIcon';

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// この端末で、既定でないアイコン (黒猫) を選んでいるとき。ヘッダーのロゴも替わる。
export const Chosen: Story = { beforeEach: () => withStorage({ [iconKey]: 'kuro' }) };

// アイコンを選び直して、保存したことを通知したとき。Story を閉じたら、選んだアイコンを戻す。
export const Saved: Story = {
  beforeEach: () => withStorage({ [iconKey]: 'default' }),
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('radio', { name: /ハチワレ/ }));
  },
};
