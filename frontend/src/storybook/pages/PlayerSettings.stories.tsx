import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// プレイヤーの設定 (/settings/player) の画面 (#2056)。端末ごとの設定は、Storybook を開いたブラウザの localStorage を読む。
const meta = {
  title: 'ページ/設定/プレイヤー',
  component: PageStory,
  args: { url: '/settings/player' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
