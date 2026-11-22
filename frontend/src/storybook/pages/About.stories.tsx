import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// Kotatsu について (/about) の画面 (#2056)。
const meta = {
  title: 'ページ/Kotatsuについて',
  component: PageStory,
  args: { url: '/about' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
