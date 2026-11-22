import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// タイムシフト (/timeshift/$serviceId) の画面 (#2056)。
const meta = {
  title: 'ページ/タイムシフト',
  component: PageStory,
  args: { url: '/timeshift/Service:1024' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
