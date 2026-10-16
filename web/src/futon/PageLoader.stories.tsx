import type { Meta, StoryObj } from '@storybook/react-vite';

import { PageLoader } from './PageLoader';

const meta = {
  title: 'futon/PageLoader',
  component: PageLoader,
} satisfies Meta<typeof PageLoader>;

export default meta;

export const Default: StoryObj<typeof meta> = {};
