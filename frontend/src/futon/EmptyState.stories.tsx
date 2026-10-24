import type { Meta, StoryObj } from '@storybook/react-vite';
import { CalendarX2, ListChecks } from 'lucide-react';

import { EmptyState } from './EmptyState';

const meta = {
  title: 'futon/EmptyState',
  component: EmptyState,
} satisfies Meta<typeof EmptyState>;

export default meta;

type Story = StoryObj<typeof meta>;

export const WithHint: Story = {
  args: {
    icon: CalendarX2,
    title: '表示できる番組がありません',
    hint: 'お使いのチューナーサーバーが正しくEPGを受信できているか確認してください。',
  },
};

export const TitleOnly: Story = {
  args: { icon: ListChecks, title: 'ルールはまだありません' },
};
