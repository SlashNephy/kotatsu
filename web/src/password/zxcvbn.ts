import { ZxcvbnFactory } from '@zxcvbn-ts/core';
import * as common from '@zxcvbn-ts/language-common';
import * as en from '@zxcvbn-ts/language-en';
import * as ja from '@zxcvbn-ts/language-ja';

// パスワードの強度の判定 (設計書 2.2)。サーバー (goja) とクライアントで、同じ辞書と同じ版を使う。
// サーバーは zxcvbn/goja.ts から作ったバンドルを読み、クライアントは入力中のフィードバックに使う。
const zxcvbn = new ZxcvbnFactory({
  dictionary: { ...common.dictionary, ...en.dictionary, ...ja.dictionary },
  graphs: common.adjacencyGraphs,
  translations: ja.translations,
});

export type Strength = {
  // score は 0〜4。サーバーは、閾値に満たないパスワードを拒否する。
  score: number;
  guessesLog10: number;
  warning: string | null;
  suggestions: string[];
};

// maxStrengthInput は、判定するパスワードの長さ (文字数) の上限。サーバーの password.MaxStrengthInput と同じ。
// zxcvbn-ts の計算量は入力の長さに対して急に増えるので、これより長いパスワードは判定せずに十分強いとみなす。
export const maxStrengthInput = 128;

export function checkStrength(password: string, userInputs: string[]): Strength {
  // サーバーと同じく、コードポイントの数で数える。
  if (Array.from(password).length > maxStrengthInput) {
    return { score: 4, guessesLog10: 0, warning: null, suggestions: [] };
  }
  const result = zxcvbn.check(password, userInputs);

  return {
    score: result.score,
    guessesLog10: result.guessesLog10,
    warning: result.feedback.warning,
    suggestions: result.feedback.suggestions,
  };
}
