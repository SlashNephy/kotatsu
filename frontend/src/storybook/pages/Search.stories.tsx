import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { failing, holdAll, PageStory } from '../PageStory';

// 番組検索 (/search) の画面 (#2056)。検索のキーワードは URL に載らないので、Story を開いた後に打ち込む。
const meta = {
  title: 'ページ/番組検索',
  component: PageStory,
  args: { url: '/search' },
  parameters: { layout: 'fullscreen' },
  play: async ({ canvas }) => {
    await userEvent.type(
      await canvas.findByRole('textbox', { name: '含めるキーワード' }),
      'こたつ',
    );
  },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

// 読み込み中は、キーワードの入力欄が出ないので、打ち込まない。
export const Loading: Story = { args: { hold: holdAll }, play: async () => {} };

// 検索に失敗したとき。
export const Error: Story = { args: { overrides: { searchPrograms: failing() } } };
