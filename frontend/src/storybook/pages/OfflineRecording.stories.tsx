import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';

// 端末に保存した録画の再生 (/play/offline/$recordingId) の画面。Storybook を開いたブラウザに保存した録画がなければ、見つからない表示になる (#2056)。
const meta = {
  title: 'ページ/オフライン/再生',
  component: PageStory,
  args: { url: '/play/offline/Recording:missing' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };
