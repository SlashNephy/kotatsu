import type { Meta, StoryObj } from '@storybook/react-vite';

import { Authorship } from './Authorship';

const meta = {
  title: 'futon/Authorship',
  component: Authorship,
  args: { createdAt: '2026-10-16T00:00:00Z', updatedAt: '2026-10-17T12:30:00Z' },
} satisfies Meta<typeof Authorship>;

export default meta;

type Story = StoryObj<typeof meta>;

export const ByUsers: Story = {
  args: { createdBy: { displayName: 'あかり' }, updatedBy: { displayName: 'ひなた' } },
};

export const Automatic: Story = { args: { createdBy: null, updatedBy: null, automatic: true } };

export const DeletedUser: Story = {
  args: { createdBy: null, updatedBy: { displayName: 'ひなた' } },
};
