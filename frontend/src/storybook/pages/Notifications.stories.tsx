import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 通知の一覧 (/notifications) の画面。デモには通知がないので、空の一覧になる (#2056)。
const meta = {
  title: 'ページ/通知',
  component: PageStory,
  args: { url: '/notifications' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
