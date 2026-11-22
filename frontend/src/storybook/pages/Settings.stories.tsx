import type { Meta, StoryObj } from '@storybook/react-vite';

import { asViewer } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// 設定の一覧 (/settings) の画面 (#2056)。
const meta = {
  title: 'ページ/設定',
  component: PageStory,
  args: { url: '/settings' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// メンバーから見たとき。管理の項目を出さない。
export const AsMember: Story = { args: { overrides: asViewer('MEMBER') } };

// 視聴者から見たとき。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };
