/// <reference types="vite/client" />
import '@mantine/core/styles.css';
import '../src/i18n';

import { MantineProvider } from '@mantine/core';
import type { Preview } from '@storybook/react-vite';

import { cssVariablesResolver, theme } from '../src/futon';

const preview: Preview = {
  decorators: [
    (Story) => (
      <MantineProvider
        theme={theme}
        cssVariablesResolver={cssVariablesResolver}
        defaultColorScheme="light"
      >
        <Story />
      </MantineProvider>
    ),
  ],
};

export default preview;
