import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// パスワードの変更 (/settings/password) の画面 (#2056)。
const meta = {
  title: 'ページ/設定/パスワード',
  component: PageStory,
  args: { url: '/settings/password' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
