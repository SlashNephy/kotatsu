import type { Meta, StoryObj } from '@storybook/react-vite';

import { paginate } from '../../demo/data';
import { failing, holdAll, PageStory } from '../PageStory';

// 予約の一覧 (/reservations) の画面 (#2056)。
const meta = {
  title: 'ページ/予約',
  component: PageStory,
  args: { url: '/reservations' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 予約が 1 つもないとき。
export const Empty: Story = {
  args: {
    overrides: {
      reservations: (args: { first: number; after?: string | null }) =>
        paginate([], args.first, args.after),
    },
  },
};

export const Error: Story = { args: { overrides: { reservations: failing() } } };
