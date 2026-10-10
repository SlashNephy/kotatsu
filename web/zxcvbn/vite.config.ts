import { defineConfig } from 'vite-plus';

// サーバーの goja で動かす zxcvbn-ts のバンドル (設計書 2.2)。mise run generate で
// internal/password/zxcvbn.js に書き出し、Go のバイナリに gzip で埋め込む。
// goja は ES2020 までなら動くことを確かめているので、それより新しい構文は変換する。
export default defineConfig({
  publicDir: false,
  build: {
    target: 'es2020',
    // web/dist は SPA の埋め込み先なので、別のディレクトリに書き出す。
    outDir: 'zxcvbn/dist',
    emptyOutDir: true,
    minify: true,
    lib: {
      entry: new URL('./goja.ts', import.meta.url).pathname,
      formats: ['iife'],
      name: 'kotatsuZxcvbn',
      fileName: () => 'zxcvbn.js',
    },
  },
});
