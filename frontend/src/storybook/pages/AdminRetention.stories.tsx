import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 保存期間 (/admin/retention) の画面 (#2056)。
const meta = {
  title: 'ページ/管理/保存期間',
  component: PageStory,
  args: { url: '/admin/retention' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
