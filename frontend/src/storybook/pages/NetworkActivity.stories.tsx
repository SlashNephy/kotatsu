import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 通信の記録 (/network-activity) の画面 (#2056)。
const meta = {
  title: 'ページ/通信の記録',
  component: PageStory,
  args: { url: '/network-activity' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
