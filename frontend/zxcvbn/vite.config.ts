import { defineConfig, type Plugin } from 'vite-plus';

import { zxcvbnDictionary } from '../src/password/dictionary';
import { precomputeDictionary } from './precompute';

const precomputedDictionaryId = 'virtual:zxcvbn-precomputed-dictionary';

// precomputedDictionary は、辞書のすべての語を並べたインデックスを、JSON の文字列として出力する。
// JS のオブジェクトのリテラルではなく文字列にするのは、goja で JSON.parse する方が速いからである。
function precomputedDictionary(): Plugin {
  return {
    name: 'kotatsu-zxcvbn-precomputed-dictionary',
    resolveId(id) {
      return id === precomputedDictionaryId ? `\0${precomputedDictionaryId}` : null;
    },
    load(id) {
      if (id !== `\0${precomputedDictionaryId}`) {
        return null;
      }
      const json = JSON.stringify(precomputeDictionary(zxcvbnDictionary));

      return `export default ${JSON.stringify(json)};`;
    },
  };
}

// サーバーの goja で動かす zxcvbn-ts のバンドル (設計書 2.2)。mise run //backend:generate で
// backend/internal/lib/password/zxcvbn.js に出力し、Go のバイナリに gzip で埋め込む。
// goja は ES2020 までなら動くことを確認しているので、それより新しい構文は変換する。
export default defineConfig({
  plugins: [precomputedDictionary()],
  publicDir: false,
  build: {
    target: 'es2020',
    // frontend/dist はデモの出力先なので、別のディレクトリに出力する。
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
