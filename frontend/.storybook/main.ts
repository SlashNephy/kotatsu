import type { StorybookConfig } from '@storybook/react-vite';

// Storybook は、futon の部品と、アプリの画面 (src/storybook/pages) の確認に使う (設計書 10.1、#2056)。
const config: StorybookConfig = {
  framework: '@storybook/react-vite',
  // 利用状況を Storybook の開発元に送らない。
  core: { disableTelemetry: true },
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
