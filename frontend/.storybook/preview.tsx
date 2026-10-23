/// <reference types="vite/client" />
import '@mantine/core/styles.css';
import '../src/futon/global.css';
import '../src/i18n';

import { MantineProvider } from '@mantine/core';
import type { Preview } from '@storybook/react-vite';

import { cssVariablesResolver, theme } from '../src/futon';

const preview: Preview = {
  // ツールバーで、ライトとダークのテーマを切り替えて見られるようにする。
  globalTypes: {
    colorScheme: {
      description: 'テーマ',
      toolbar: {
        title: 'テーマ',
        icon: 'mirror',
        items: [
          { value: 'light', title: 'ライト' },
          { value: 'dark', title: 'ダーク' },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { colorScheme: 'light' },
  decorators: [
    (Story, context) => (
      <MantineProvider
        theme={theme}
        cssVariablesResolver={cssVariablesResolver}
        forceColorScheme={context.globals['colorScheme'] === 'dark' ? 'dark' : 'light'}
      >
        <Story />
      </MantineProvider>
    ),
  ],
};

export default preview;
