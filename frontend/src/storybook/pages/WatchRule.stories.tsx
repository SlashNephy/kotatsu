import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 視聴ルールの編集 (/rules/watch/$watchRuleId) の画面。デモには視聴ルールがないので、見つからない表示になる (#2056)。
const meta = {
  title: 'ページ/ルール/視聴ルール',
  component: PageStory,
  args: { url: '/rules/watch/WatchRule:missing' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
