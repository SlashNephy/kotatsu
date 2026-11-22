import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// ライブ視聴 (/live/\$serviceId) の画面。Storybook では映像を流さないので、操作の部品と、映像を読めないときの表示を見る (#2056)。
const meta = {
  title: 'ページ/ライブ/視聴',
  component: PageStory,
  args: { url: '/live/Service:1024' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
