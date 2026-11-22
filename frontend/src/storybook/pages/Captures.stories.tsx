import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, holdAll, PageStory } from '../PageStory';

// キャプチャの一覧 (/captures) の画面 (#2056)。
const meta = {
  title: 'ページ/キャプチャ',
  component: PageStory,
  args: { url: '/captures' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { captures: failing() } } };
