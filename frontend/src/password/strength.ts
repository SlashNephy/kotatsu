import type { ZxcvbnFactory } from '@zxcvbn-ts/core';
import { adjacencyGraphs } from '@zxcvbn-ts/language-common';
import { translations } from '@zxcvbn-ts/language-ja';

// パスワードの強度の判定 (設計書 2.2) のうち、サーバー (goja) とクライアントで共通の部分。
// 辞書の持ち方だけが違う。クライアントは辞書をそのまま渡し、サーバーは順位を付け終えた辞書を埋め込む。

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

// zxcvbnOptions は、辞書のほかの設定。
export const zxcvbnOptions = { graphs: adjacencyGraphs, translations };

export function checkStrengthWith(
  zxcvbn: ZxcvbnFactory,
  password: string,
  userInputs: string[],
): Strength {
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
