import { Options, type ZxcvbnFactory } from '@zxcvbn-ts/core';

export type PrecomputedDictionary = NonNullable<
  NonNullable<ConstructorParameters<typeof ZxcvbnFactory>[0]>['precomputedDictionary']
>;

// precomputeDictionary は、辞書のすべての語を並べたインデックスを、JSON にできる形で返す (設計書 2.2)。
// インデックスの作り方は zxcvbn-ts の Options に任せるので、クライアントで作るインデックスと同じになる。
export function precomputeDictionary(
  dictionary: Record<string, (string | number)[]>,
): PrecomputedDictionary {
  const { wordIndex, dictionaryMaxWordSizes } = new Options({ dictionary });

  return {
    wordIndex: {
      dictionaryNames: wordIndex.dictionaryNames,
      // listSizes は型の上では private だが、インデックスを作り直すのに要る。
      listSizes: (wordIndex as unknown as { listSizes: number[] }).listSizes,
      words: wordIndex.words,
      hitStart: Array.from(wordIndex.hitStart),
      hitDictionary: Array.from(wordIndex.hitDictionary),
      hitRank: Array.from(wordIndex.hitRank),
    },
    dictionaryMaxWordSizes,
  };
}
