import { ZxcvbnFactory } from '@zxcvbn-ts/core';
import { describe, expect, test } from 'vitest';

import { precomputeDictionary, type PrecomputedDictionary } from '../../zxcvbn/precompute';
import { zxcvbnDictionary } from './dictionary';
import { checkStrengthWith, zxcvbnOptions } from './strength';

// サーバー (goja) は、並べ終えた辞書のインデックスを precomputedDictionary で渡す (zxcvbn/goja.ts)。
// クライアントのように辞書をそのまま渡した場合と、同じ判定になることを確認する。
describe('precomputedDictionary', () => {
  const withDictionary = new ZxcvbnFactory({ ...zxcvbnOptions, dictionary: zxcvbnDictionary });
  const precomputed = new ZxcvbnFactory({
    ...zxcvbnOptions,
    precomputedDictionary: JSON.parse(
      JSON.stringify(precomputeDictionary(zxcvbnDictionary)),
    ) as PrecomputedDictionary,
  });

  test.each([
    { name: 'よく使われるパスワード', password: 'password', userInputs: [] },
    { name: 'l33t', password: 'P4$$w0rdP4$$w0rd', userInputs: [] },
    { name: '日本語の辞書', password: 'kotatsu2026', userInputs: [] },
    { name: 'ユーザー名', password: 'spica1234', userInputs: ['spica'] },
    { name: '曜日の並び', password: 'mondaytuesday', userInputs: [] },
    {
      name: '記号の多い長いパスワード',
      password: 'q7#Vt9!mZ2@xLp4$Rk8&Wn3*Hj6^Bd5%Fs1(Gc0)Ya2',
      userInputs: [],
    },
  ])('$name', ({ password, userInputs }) => {
    expect(checkStrengthWith(precomputed, password, userInputs)).toEqual(
      checkStrengthWith(withDictionary, password, userInputs),
    );
  });
});
