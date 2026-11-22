import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 番組表の設定 (/settings/guide) の画面 (#2056)。
const meta = {
  title: 'ページ/設定/番組表',
  component: PageStory,
  args: { url: '/settings/guide' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
