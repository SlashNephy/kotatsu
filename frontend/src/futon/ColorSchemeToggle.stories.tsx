import type { Meta, StoryObj } from '@storybook/react-vite';

import { ColorSchemeToggle } from './ColorSchemeToggle';

const meta = {
  title: 'futon/ColorSchemeToggle',
  component: ColorSchemeToggle,
} satisfies Meta<typeof ColorSchemeToggle>;

export default meta;

export const Default: StoryObj<typeof meta> = {};
