import { Options } from '@zxcvbn-ts/core';
import { defineConfig, type Plugin } from 'vite-plus';

import { zxcvbnDictionary } from '../src/password/dictionary';

const rankedDictionaryId = 'virtual:zxcvbn-ranked-dictionary';

// wordBigrams は、辞書の語に現れる 2 文字の組 (UTF-16 の 2 単位) を、重ねずに並べる。
// 辞書の照合は、どの語にも現れない組を含む部分を引かずに済ませる (設計書 2.2)。
function wordBigrams(dictionaries: Record<string, Record<string, number>>): string[] {
  const bigrams = new Set<string>();
  for (const dictionary of Object.values(dictionaries)) {
    for (const word of Object.keys(dictionary)) {
      for (let k = 0; k + 1 < word.length; k += 1) {
        bigrams.add(word.slice(k, k + 2));
      }
    }
  }

  return [...bigrams].sort();
}

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
        wordBigrams: wordBigrams(options.rankedDictionaries),
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
