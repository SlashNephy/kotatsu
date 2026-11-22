import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// セキュリティの設定 (/settings/security) の画面 (#2056)。
const meta = {
  title: 'ページ/設定/セキュリティ',
  component: PageStory,
  args: { url: '/settings/security' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
