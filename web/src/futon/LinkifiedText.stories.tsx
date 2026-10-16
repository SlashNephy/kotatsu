import type { Meta, StoryObj } from '@storybook/react-vite';

import { LinkifiedText } from './LinkifiedText';

const meta = {
  title: 'futon/LinkifiedText',
  component: LinkifiedText,
} satisfies Meta<typeof LinkifiedText>;

export default meta;

export const WithURL: StoryObj<typeof meta> = {
  args: { text: '番組のホームページ https://www.example.com/program をご覧ください。' },
};
