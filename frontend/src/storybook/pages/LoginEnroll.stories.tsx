import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 認証アプリの登録 (/login/enroll) の画面 (#2056)。
const meta = {
  title: 'ページ/ログイン/認証アプリの登録',
  component: PageStory,
  args: { url: '/login/enroll' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
