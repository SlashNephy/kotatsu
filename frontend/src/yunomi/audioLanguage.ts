import { useSyncExternalStore } from 'react';

import type { KyusuAudioChoice } from './audio';
import type { KyusuAudio } from './route';
import { browserStorage } from './volume';

// 音声の言語の優先順位 (#819、設計書 7.5)。二か国語の番組で、いつも英語を選ぶ、などを端末ごとに覚える。
// 利用者がメニューで音声を選ぶまでは、番組ごとに、優先する言語の音声を選ぶ。

const storageKey = 'kotatsu.player.audioLanguage';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:audio-language';

// audioLanguages は、優先する言語に選べる、記述子の言語のコード (ISO 639-2)。
export const audioLanguages = ['jpn', 'eng', 'kor', 'zho'] as const;

export type AudioLanguage = (typeof audioLanguages)[number];

// languageAliases は、同じ言語を表すほかのコード。中国語は、ISO 639-2 の B 形式 (chi) で書く局もある。
const languageAliases: Readonly<Record<AudioLanguage, readonly string[]>> = {
  jpn: ['jpn'],
  eng: ['eng'],
  kor: ['kor'],
  zho: ['zho', 'chi'],
};

// loadAudioLanguage は、優先する言語を読む。決めていないか、読めなければ null を返す。
export function loadAudioLanguage(storage: Pick<Storage, 'getItem'> | null): AudioLanguage | null {
  try {
    const value = storage?.getItem(storageKey);

    return audioLanguages.find((language) => language === value) ?? null;
  } catch {
    return null;
  }
}

// preferredSlot は、番組の音声の選択肢 choices から、優先する言語 language の音声の番号を返す。
// 選べる音声が 2 つ以上なく、言語の合うものがなければ null を返す。
export function preferredSlot(
  choices: readonly KyusuAudioChoice[],
  language: AudioLanguage | null,
): KyusuAudio | null {
  if (language === null) {
    return null;
  }
  const codes = languageAliases[language];

  return (
    choices.find((choice) => choice.language !== null && codes.includes(choice.language))?.slot ??
    null
  );
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useAudioLanguage は、優先する言語と、それを変える関数を返す。ほかのタブで変えても追う。
export function useAudioLanguage(): [
  AudioLanguage | null,
  (language: AudioLanguage | null) => void,
] {
  const language = useSyncExternalStore(subscribe, () => loadAudioLanguage(browserStorage()));

  return [
    language,
    (next) => {
      try {
        if (next === null) {
          browserStorage()?.removeItem(storageKey);
        } else {
          browserStorage()?.setItem(storageKey, next);
        }
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
