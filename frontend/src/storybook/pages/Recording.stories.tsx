import type { Meta, StoryObj } from '@storybook/react-vite';

import { firstRecordingPath, holdAll, PageStory } from '../PageStory';

// 録画の詳細 (/recordings/\$recordingId) の画面 (#2056)。
const meta = {
  title: 'ページ/録画/詳細',
  component: PageStory,
  args: { url: firstRecordingPath('/recordings/:id') },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 録画が見つからないとき (消した録画の URL を開いたなど)。
export const NotFound: Story = {
  args: { url: () => Promise.resolve('/recordings/Recording:missing') },
};
