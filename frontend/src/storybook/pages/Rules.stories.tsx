import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, holdAll, PageStory } from '../PageStory';

// ルールの一覧 (/rules) の画面 (#2056)。
const meta = {
  title: 'ページ/ルール',
  component: PageStory,
  args: { url: '/rules' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ルールがまだ 1 つもないとき。
export const Empty: Story = { args: { overrides: { rules: () => [] } } };

export const Error: Story = { args: { overrides: { rules: failing() } } };
