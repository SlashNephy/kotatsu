import { Options } from '@zxcvbn-ts/core';
import { defineConfig, type Plugin } from 'vite-plus';

import { zxcvbnDictionary } from '../src/password/dictionary';

const rankedDictionaryId = 'virtual:zxcvbn-ranked-dictionary';

// rankedDictionary は、辞書に順位を付け終えたものを、JSON の文字列として書き出す。
// 順位の付け方は zxcvbn-ts の Options に任せるので、クライアントで付ける順位と同じになる。
// JS のオブジェクトのリテラルではなく文字列にするのは、goja で JSON.parse する方が速いからである。
function rankedDictionary(): Plugin {
  return {
    name: 'kotatsu-zxcvbn-ranked-dictionary',
    resolveId(id) {
      return id === rankedDictionaryId ? `\0${rankedDictionaryId}` : null;
    },
    load(id) {
      if (id !== `\0${rankedDictionaryId}`) {
        return null;
      }
      const options = new Options({ dictionary: zxcvbnDictionary });
      const json = JSON.stringify({
        rankedDictionaries: options.rankedDictionaries,
        rankedDictionariesMaxWordSize: options.rankedDictionariesMaxWordSize,
      });

      return `export default ${JSON.stringify(json)};`;
    },
  };
}

// サーバーの goja で動かす zxcvbn-ts のバンドル (設計書 2.2)。mise run generate で
// internal/password/zxcvbn.js に書き出し、Go のバイナリに gzip で埋め込む。
// goja は ES2020 までなら動くことを確かめているので、それより新しい構文は変換する。
export default defineConfig({
  plugins: [rankedDictionary()],
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
