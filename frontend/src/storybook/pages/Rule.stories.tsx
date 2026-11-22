import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ルールの編集 (/rules/$ruleId) の画面 (#2056)。
const meta = {
  title: 'ページ/ルール/編集',
  component: PageStory,
  args: { url: '/rules/Rule:1' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ルールが見つからないとき (消されたルールの URL を開いたなど)。
export const NotFound: Story = { args: { url: '/rules/Rule:999' } };
