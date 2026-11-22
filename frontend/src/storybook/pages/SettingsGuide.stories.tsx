import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { withStorage } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// 番組表の設定 (/settings/guide) の画面 (#2056)。既定の見た目、この端末で変えた見た目、変えて保存を知らせたときを並べる。
const meta = {
  title: 'ページ/設定/番組表',
  component: PageStory,
  args: { url: '/settings/guide' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// appearanceKey は、番組表の見た目を覚える localStorage のキー (src/guide/appearance.ts)。
const appearanceKey = 'kotatsu.guide.appearance';

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// この端末で、詰めた表示、通販番組の灰色、ジャンルの色を変えているとき。
export const Customized: Story = {
  beforeEach: () =>
    withStorage({
      [appearanceKey]: JSON.stringify({
        density: 'compact',
        grayShopping: true,
        genreColors: { 0: 'red', 1: 'violet', 7: 'pink' },
      }),
    }),
};

// 設定を変えて、保存したことを知らせたとき。Story を閉じたら、変えた設定を戻す。
export const Saved: Story = {
  beforeEach: () => withStorage({ [appearanceKey]: JSON.stringify({}) }),
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('switch', { name: /通販番組を灰色で出す/ }));
  },
};
