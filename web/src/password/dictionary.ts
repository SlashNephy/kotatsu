import * as common from '@zxcvbn-ts/language-common';
import * as en from '@zxcvbn-ts/language-en';
import * as ja from '@zxcvbn-ts/language-ja';

// zxcvbnDictionary は、判定に使う辞書。サーバーの埋め込みは、ビルドのときにこれから順位を付けて作る。
export const zxcvbnDictionary = { ...common.dictionary, ...en.dictionary, ...ja.dictionary };
