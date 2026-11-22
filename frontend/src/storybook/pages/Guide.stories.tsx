import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, holdAll, PageStory } from '../PageStory';

// 番組表 (/guide) の画面 (#2056)。
const meta = {
  title: 'ページ/番組表',
  component: PageStory,
  args: { url: '/guide' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

// 読み込み中。tenban が応答を返すまでの画面。
export const Loading: Story = { args: { hold: holdAll } };

// 番組表を読めなかったとき。
export const Error: Story = { args: { overrides: { guide: failing() } } };
