import type { StorybookConfig } from '@storybook/react-vite';

import { demoStream } from '../demo/plugin.ts';

// Storybook は、futon の部品と、アプリの画面 (src/storybook/pages) の確認に使う (設計書 10.1、#2056)。
const config: StorybookConfig = {
  framework: '@storybook/react-vite',
  // 利用状況を Storybook の開発元に送らない。
  core: { disableTelemetry: true },
  stories: ['../src/**/*.stories.tsx'],
  // 画面の Story で、アプリと同じ静的なファイル (ブラウザの中で変換する WebAssembly、アイコンなど) を読めるようにする。
  staticDirs: ['../public'],
  viteFinal: (viteConfig) => ({
    ...viteConfig,
    // ブラウザの中で変換する Worker は、WebAssembly を BASE_URL から読む。Storybook の既定の相対の base (./) では、Worker の
    // 置き場所 (assets/) から探して見つからないので、アプリと同じく / に置く。
    base: '/',
    // Storybook には、ルーティングの生成と Service Worker の生成が要らない。
    // 画面の Story で、ライブ視聴と録画の再生にデモの映像を流せるよう、静的なデモと同じ Service Worker と映像を配る。
    plugins: [
      ...(viteConfig.plugins ?? []).flat().filter((plugin) => {
        const name = plugin && typeof plugin === 'object' && 'name' in plugin ? plugin.name : '';

        return !name.startsWith('vite-plugin-pwa') && !name.startsWith('tanstack');
      }),
      demoStream(),
    ],
  }),
};

export default config;
