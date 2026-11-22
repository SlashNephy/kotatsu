import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 通知の設定 (/settings/notifications) の画面 (#2056)。
const meta = {
  title: 'ページ/設定/通知',
  component: PageStory,
  args: { url: '/settings/notifications' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
