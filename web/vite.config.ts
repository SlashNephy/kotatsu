import babel from '@rolldown/plugin-babel';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react, { reactCompilerPreset } from '@vitejs/plugin-react';
import { defineConfig, lazyPlugins } from 'vite-plus';

// ビルドした SPA は、Go のバイナリに埋め込む (設計書 2.3)。web/embed.go が dist/app を埋め込む。
// dist 自体は、未ビルドでも go build が通るように、空の .gitkeep だけを置いてリポジトリに残す。
export default defineConfig({
  plugins: lazyPlugins(() => [
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
    // graphql`...` を、Relay のコンパイラが生成したアーティファクトの import に置き換える。
    babel({ presets: [reactCompilerPreset()], plugins: [['relay', { eagerEsModules: true }]] }),
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
    ignorePatterns: ['src/routeTree.gen.ts', 'dist/**', '**/__generated__/**'],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  test: {
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
