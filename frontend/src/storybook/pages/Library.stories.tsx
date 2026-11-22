import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ライブラリ (/library) の画面。デモにはライブラリがないので、空の一覧になる (#2056)。
const meta = {
  title: 'ページ/ライブラリ',
  component: PageStory,
  args: { url: '/library' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
