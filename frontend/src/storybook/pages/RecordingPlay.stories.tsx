import type { Meta, StoryObj } from '@storybook/react-vite';

import { firstRecordingPath, holdAll, PageStory } from '../PageStory';

// 録画の再生 (/recordings/\$recordingId/play) の画面。Storybook では映像を流さないので、操作の部品と、映像を読めないときの表示を見る (#2056)。
const meta = {
  title: 'ページ/録画/再生',
  component: PageStory,
  args: { url: firstRecordingPath('/recordings/:id/play') },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
