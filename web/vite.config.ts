import { readFileSync } from 'node:fs';

import babel from '@rolldown/plugin-babel';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react, { reactCompilerPreset } from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';
import { defineConfig, lazyPlugins, type Plugin } from 'vite-plus';

// ビルドした SPA は、Go のバイナリに埋め込む (設計書 2.3)。web/embed.go が dist/app を埋め込む。
// dist 自体は、未ビルドでも go build が通るように、空の .gitkeep だけを置いてリポジトリに残す。
// devAPI は、開発サーバーが API を中継する先。
// 開発用の Compose では、KOTATSU_DEV_API で Kotatsu のコンテナを指す。
// Compose の外で mise run web:dev を動かすときは、手元で起動した Kotatsu (既定は 127.0.0.1:8080) に中継する。
// 空の値は、渡していないものとして扱う。
const devAPI =
  process.env.KOTATSU_DEV_API || `http://127.0.0.1:${process.env.KOTATSU_DEV_PORT || '8080'}`;

// inlineAppleTouchIcon は、index.html の apple-touch-icon を、public/icons の PNG の data: URL に置き換える (#582)。
// iOS 27 の「ホーム画面に追加」は、URL で指したアイコンを読み込めず、タイトルの頭文字のタイルを出す。
// data: URL なら、別のリクエストなしにアイコンを渡せる。
function inlineAppleTouchIcon(): Plugin {
  const placeholder = '__KOTATSU_APPLE_TOUCH_ICON__';

  return {
    name: 'kotatsu:inline-apple-touch-icon',
    transformIndexHtml(html) {
      const icon = readFileSync(
        new URL('./public/icons/apple-touch-icon-180.png', import.meta.url),
      );

      return html.replace(placeholder, `data:image/png;base64,${icon.toString('base64')}`);
    },
  };
}

// mode が demo のときは、静的なデモ (#832) をビルドする。`vp build --mode demo` で使う。
// demoStream は、静的なデモ (#832) のときだけ、合成の映像 (demo/stream.m2ts) と、
// ライブ視聴の映像を実時間の速さで流す Service Worker (stream-sw.js) を出力に加える。
// public に置くと、tenban に埋め込む通常のビルドにも入るので、ここで加える。
// Service Worker は、デモの全体を受け持てるよう、base の直下に置く。
function demoStream(): Plugin {
  const files = [
    { fileName: 'demo/stream.m2ts', source: './demo/stream.m2ts', type: 'video/mp2t' },
    { fileName: 'stream-sw.js', source: './demo/stream-sw.js', type: 'text/javascript' },
  ];

  return {
    name: 'kotatsu:demo-stream',
    // 開発サーバー (vp dev --mode demo) でも、同じパスで配信する。
    configureServer(server) {
      const base = server.config.base;
      server.middlewares.use((request, response, next) => {
        const path = (request.url ?? '').split('?')[0];
        const file = files.find((item) => path === `${base}${item.fileName}`);
        if (file === undefined) {
          next();

          return;
        }
        response.setHeader('Content-Type', file.type);
        response.end(readFileSync(new URL(file.source, import.meta.url)));
      });
    },
    generateBundle() {
      for (const file of files) {
        this.emitFile({
          type: 'asset',
          fileName: file.fileName,
          source: readFileSync(new URL(file.source, import.meta.url)),
        });
      }
    },
  };
}

export default defineConfig(({ mode }) => ({
  plugins: lazyPlugins(() => [
    // Relay のコンパイラが routes/ に生成する __generated__ は、ルートではない。
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routeFileIgnorePattern: '__generated__',
    }),
    react(),
    inlineAppleTouchIcon(),
    ...(mode === 'demo' ? [demoStream()] : []),
    // graphql`...` を、Relay のコンパイラが生成したアーティファクトの import に置き換える。
    babel({ presets: [reactCompilerPreset()], plugins: [['relay', { eagerEsModules: true }]] }),
    // PWA の外枠 (設計書 10.1)。Web Push の受信とナビゲーションは、自前の Service Worker (src/sw.ts) に書く。
    // デモは Service Worker を登録しないので、作らない。
    VitePWA({
      disable: mode === 'demo',
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      // 登録は main.tsx で virtual:pwa-register から行う。CSP の nonce のない inline script を差し込ませない。
      injectRegister: false,
      injectManifest: {
        // 外枠だけを対象にする。見出しのフォントは、unicode-range ごとに 100 以上のファイルに分かれて
        // 3 MB ほどになるので含めない。オフラインでは、見出しもシステムフォントで出る。
        globPatterns: ['**/*.{js,css,html,png,svg,webmanifest}'],
        // パスワードの強度の判定の辞書 (2 MB ほど) は、セットアップとパスワードの変更でしか使わないので含めない。
        globIgnores: ['**/zxcvbn-*.js'],
      },
      manifest: {
        name: 'Kotatsu',
        short_name: 'Kotatsu',
        description: 'テレビの予約、録画、視聴をまとめるアプリ',
        lang: 'ja',
        // マニフェストの URL から解決させ、Vite の base に置いても同じ場所を指すようにする (#832)。
        start_url: '.',
        scope: '.',
        display: 'standalone',
        theme_color: '#ff7407',
        background_color: '#fbf7ef',
        icons: [
          { src: 'icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          {
            src: 'icons/icon-maskable-512.png',
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
    // 開発中は、API を手元の tenban に中継する。compose.yaml と同じく、KOTATSU_DEV_PORT でポートを変えられる。
    // Host は書き換えない。tenban は、Host から求めたオリジンと Origin を比べて /auth/* の CSRF を防ぎ、
    // OIDC のリダイレクト URI も Host から作る。文字列で書くと changeOrigin が有効になり、どちらも食い違う。
    proxy: {
      '/graphql': { target: devAPI, changeOrigin: false },
      '/auth': { target: devAPI, changeOrigin: false },
      // 映像、録画の画像、ロゴも中継し、開発サーバーだけで画面のすべてを確かめられるようにする。
      '/streams': { target: devAPI, changeOrigin: false },
      '/media': { target: devAPI, changeOrigin: false },
      // EPGStation 互換の API (#431) も、開発サーバーから確かめられるようにする。
      '/api': { target: devAPI, changeOrigin: false },
      // MCP のクライアントにトークンを発行する認可サーバー (#963) も、開発サーバーから確かめられるようにする。
      '/oauth': { target: devAPI, changeOrigin: false },
      '/.well-known': { target: devAPI, changeOrigin: false },
    },
  },
  fmt: {
    singleQuote: true,
    // vendor/ は、上流のソースからビルドした成果物なので、整形しない (scripts/build-mpeg2toh264.sh)。
    ignorePatterns: [
      'src/routeTree.gen.ts',
      // internal/epg/printenclosed が書き出す (mise run generate)。
      'src/futon/enclosedCharacters.gen.ts',
      'schema.graphql',
      '**/__generated__/**',
      'vendor/**',
    ],
  },
  lint: {
    ignorePatterns: [
      'src/routeTree.gen.ts',
      'dist/**',
      'storybook-static/**',
      '**/__generated__/**',
      'vendor/**',
    ],
    options: {
      typeAware: true,
      typeCheck: true,
    },
    rules: {
      // useLazyLoadQuery は、ErrorBoundary が戻したときに読み直す relay/useLazyLoadQuery を使う。
      'no-restricted-imports': [
        'error',
        {
          paths: [
            {
              name: 'react-relay',
              importNames: ['useLazyLoadQuery'],
              message: 'Use useLazyLoadQuery from src/relay/useLazyLoadQuery instead.',
            },
          ],
        },
      ],
    },
    overrides: [
      {
        files: ['src/relay/useLazyLoadQuery.ts'],
        rules: { 'no-restricted-imports': 'off' },
      },
    ],
  },
  test: {
    include: ['src/**/*.test.{ts,tsx}'],
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
}));
