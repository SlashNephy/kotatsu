import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// オフラインで見る録画 (/offline) の画面。端末に保存した録画は、Storybook を開いたブラウザのものを読む (#2056)。
const meta = {
  title: 'ページ/オフライン',
  component: PageStory,
  args: { url: '/offline' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
