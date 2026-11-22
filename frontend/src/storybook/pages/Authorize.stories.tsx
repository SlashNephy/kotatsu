import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// アプリとの連携の許可 (/authorize) の画面 (#2056)。
const meta = {
  title: 'ページ/連携の許可',
  component: PageStory,
  args: { url: '/authorize' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
