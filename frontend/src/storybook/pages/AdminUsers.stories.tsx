import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ユーザーの管理 (/admin/users) の画面 (#2056)。
const meta = {
  title: 'ページ/管理/ユーザー',
  component: PageStory,
  args: { url: '/admin/users' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
