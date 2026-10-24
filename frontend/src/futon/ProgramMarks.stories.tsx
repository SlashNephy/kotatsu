import type { Meta, StoryObj } from '@storybook/react-vite';

import { ProgramTitle } from './ProgramMarks';

const meta = {
  title: 'futon/ProgramTitle',
  component: ProgramTitle,
} satisfies Meta<typeof ProgramTitle>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Plain: Story = { args: { name: 'ニュース' } };

export const WithMarks: Story = { args: { name: '科捜研の女１６　＃５🈖🈑🈞' } };

export const EnglishMarks: Story = { args: { name: '世界遺産🆞🆠🅊' } };

export const Hoka: Story = { args: { name: 'クレヨンしんちゃん　【流れ板しんのすけ】🈀🈓🈖🈑' } };
