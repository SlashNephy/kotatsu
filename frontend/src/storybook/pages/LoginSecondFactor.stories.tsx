import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 2 段階認証 (/login/second-factor) の画面 (#2056)。
const meta = {
  title: 'ページ/ログイン/2段階認証',
  component: PageStory,
  args: { url: '/login/second-factor' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
