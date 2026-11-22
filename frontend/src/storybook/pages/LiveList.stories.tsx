import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ライブ視聴のサービスの一覧 (/live) の画面 (#2056)。
const meta = {
  title: 'ページ/ライブ',
  component: PageStory,
  args: { url: '/live' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
