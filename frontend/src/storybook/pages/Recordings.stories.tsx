import type { Meta, StoryObj } from '@storybook/react-vite';

import { paginate } from '../../demo/data';
import { failing, holdAll, PageStory } from '../PageStory';

// 録画の一覧 (/recordings) の画面 (#2056)。
const meta = {
  title: 'ページ/録画',
  component: PageStory,
  args: { url: '/recordings' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 録画がまだ 1 つもないとき。
export const Empty: Story = {
  args: {
    overrides: {
      recordings: (args: { first: number; after?: string | null }) =>
        paginate([], args.first, args.after),
    },
  },
};

export const Error: Story = { args: { overrides: { recordings: failing() } } };
