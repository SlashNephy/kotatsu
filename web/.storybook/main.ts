import type { StorybookConfig } from '@storybook/react-vite';

// Storybook は futon の部品の確認に使う (設計書 10.1)。
const config: StorybookConfig = {
  framework: '@storybook/react-vite',
  stories: ['../src/**/*.stories.tsx'],
  viteFinal: (viteConfig) => ({
    ...viteConfig,
    // Storybook には、ルーティングの生成と Service Worker の生成が要らない。
    plugins: (viteConfig.plugins ?? []).flat().filter((plugin) => {
      const name = plugin && typeof plugin === 'object' && 'name' in plugin ? plugin.name : '';

      return !name.startsWith('vite-plugin-pwa') && !name.startsWith('tanstack');
    }),
  }),
};

export default config;
