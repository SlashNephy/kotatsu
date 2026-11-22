import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ログイン中の端末 (/settings/sessions) の画面 (#2056)。
const meta = {
  title: 'ページ/設定/セッション',
  component: PageStory,
  args: { url: '/settings/sessions' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
