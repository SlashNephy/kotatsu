import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, holdAll, PageStory } from '../PageStory';

// サービスごとの番組表 (/guide/channel/$serviceId) の画面 (#2056)。
const meta = {
  title: 'ページ/番組表/サービスごと',
  component: PageStory,
  args: { url: '/guide/channel/Service:1024' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { guide: failing() } } };
