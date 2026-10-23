import type { Meta, StoryObj } from '@storybook/react-vite';

import { PasswordStrength } from './PasswordStrength';

const meta = {
  title: 'futon/PasswordStrength',
  component: PasswordStrength,
  args: { userInputs: ['kotatsu'] },
} satisfies Meta<typeof PasswordStrength>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Weak: Story = { args: { password: 'kotatsu123' } };

export const Strong: Story = { args: { password: 'みかんと猫とこたつの冬休み' } };
