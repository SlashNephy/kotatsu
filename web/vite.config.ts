import babel from '@rolldown/plugin-babel';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react, { reactCompilerPreset } from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';
import { defineConfig, lazyPlugins } from 'vite-plus';

// ビルドした SPA は、Go のバイナリに埋め込む (設計書 2.3)。web/embed.go が dist/app を埋め込む。
// dist 自体は、未ビルドでも go build が通るように、空の .gitkeep だけを置いてリポジトリに残す。
export default defineConfig({
  plugins: lazyPlugins(() => [
    // Relay のコンパイラが routes/ に生成する __generated__ は、ルートではない。
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routeFileIgnorePattern: '__generated__',
    }),
    react(),
    // graphql`...` を、Relay のコンパイラが生成したアーティファクトの import に置き換える。
    babel({ presets: [reactCompilerPreset()], plugins: [['relay', { eagerEsModules: true }]] }),
    // PWA の外枠 (設計書 10.1)。Web Push の受信とナビゲーションは、自前の Service Worker (src/sw.ts) に書く。
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      // 登録は main.tsx で virtual:pwa-register から行う。CSP の nonce のない inline script を差し込ませない。
      injectRegister: false,
      injectManifest: {
        // 外枠だけを対象にする。見出しのフォントは、unicode-range ごとに 100 以上のファイルに分かれて
        // 3 MB ほどになるので含めない。オフラインでは、見出しもシステムフォントで出る。
        globPatterns: ['**/*.{js,css,html,png,svg,webmanifest}'],
      },
      manifest: {
        name: 'Kotatsu',
        short_name: 'Kotatsu',
        description: 'テレビの予約、録画、視聴をまとめるアプリ',
        lang: 'ja',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        theme_color: '#ff7407',
        background_color: '#fbf7ef',
        icons: [
          { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          {
            src: '/icons/icon-maskable-512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
    }),
  ]),
  html: {
    // tenban が index.html を配信するたびに、リクエストごとの nonce に置き換える (設計書 10.4)。
    cspNonce: '__KOTATSU_CSP_NONCE__',
  },
  build: {
    outDir: 'dist/app',
    emptyOutDir: true,
    // CSP の font-src は 'self' だけなので、小さいフォントも data: URL にせず、ファイルのまま配信する。
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
  },
  server: {
    // 開発中は、API を手元の tenban に中継する。
    proxy: {
      '/graphql': 'http://127.0.0.1:8080',
      '/auth': 'http://127.0.0.1:8080',
    },
  },
  fmt: {
    singleQuote: true,
    ignorePatterns: ['src/routeTree.gen.ts', 'schema.graphql', '**/__generated__/**'],
  },
  lint: {
    ignorePatterns: [
      'src/routeTree.gen.ts',
      'dist/**',
      'storybook-static/**',
      '**/__generated__/**',
    ],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  test: {
    include: ['src/**/*.test.{ts,tsx}'],
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
});
