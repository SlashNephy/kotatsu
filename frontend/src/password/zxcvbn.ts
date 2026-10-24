import { ZxcvbnFactory } from '@zxcvbn-ts/core';

import { zxcvbnDictionary } from './dictionary';
import { checkStrengthWith, type Strength, zxcvbnOptions } from './strength';

// クライアントで、入力中のフィードバックに使う判定 (設計書 2.2)。
// サーバー (goja) は、同じ辞書に順位を付け終えたものを使う (zxcvbn/goja.ts)。
const zxcvbn = new ZxcvbnFactory({ ...zxcvbnOptions, dictionary: zxcvbnDictionary });

export type { Strength };

export function checkStrength(password: string, userInputs: string[]): Strength {
  return checkStrengthWith(zxcvbn, password, userInputs);
}
