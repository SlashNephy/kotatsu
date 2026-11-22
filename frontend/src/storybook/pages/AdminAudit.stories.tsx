import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 監査ログ (/admin/audit) の画面 (#2056)。
const meta = {
  title: 'ページ/管理/監査ログ',
  component: PageStory,
  args: { url: '/admin/audit' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
