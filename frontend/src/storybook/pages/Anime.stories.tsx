import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, holdAll, PageStory } from '../PageStory';

// 視聴管理のアニメ (/anime) の画面 (#2056)。
const meta = {
  title: 'ページ/アニメ',
  component: PageStory,
  args: { url: '/anime' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { animeLibrary: failing() } } };
