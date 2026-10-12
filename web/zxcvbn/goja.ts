import { ZxcvbnFactory } from '@zxcvbn-ts/core';
import rankedDictionary from 'virtual:zxcvbn-ranked-dictionary';

import { checkStrengthWith, zxcvbnOptions } from '../src/password/strength';

// goja から呼ぶ入口。Go は、入力を JSON の文字列で渡し、結果を JSON の文字列で受け取る。
// goja のオブジェクトの受け渡しより、JSON の方が型の食い違いを起こしにくい。
declare global {
  var kotatsuCheckStrength: (input: string) => string;
}

// 辞書は、ビルドのときに順位を付け終えたものを JSON の文字列で埋め込む。
// goja の JSON.parse は Go で動くので、辞書を展開して順位を付け直すより、ずっと速い (設計書 2.2)。
const zxcvbn = new ZxcvbnFactory({
  ...zxcvbnOptions,
  precomputedDictionary: JSON.parse(rankedDictionary) as {
    rankedDictionaries: Record<string, Record<string, number>>;
    rankedDictionariesMaxWordSize: Record<string, number>;
  },
});

globalThis.kotatsuCheckStrength = (input: string): string => {
  const { password, userInputs } = JSON.parse(input) as { password: string; userInputs: string[] };

  return JSON.stringify(checkStrengthWith(zxcvbn, password, userInputs));
};
