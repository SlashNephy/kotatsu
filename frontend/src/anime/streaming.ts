import { useSyncExternalStore } from 'react';

import { preferenceChanged } from '../settings/syncedPreferences';

// 配信サービスで作品を探すリンク (設計書 3.8、#1235)。録画がない話を配信で見るときのため、作品名で検索する URL を作る。
// URL の形は annict-tracker と同じにする。

// StreamingService は、作品を探せる配信サービス。
export type StreamingService =
  | 'danime'
  | 'danimeNiconico'
  | 'abema'
  | 'netflix'
  | 'primeVideo'
  | 'niconicoChannel'
  | 'bandaiChannel'
  | 'youtube';

// streamingServices は、配信サービスの並び。
export const streamingServices: readonly StreamingService[] = [
  'danime',
  'danimeNiconico',
  'abema',
  'netflix',
  'primeVideo',
  'niconicoChannel',
  'bandaiChannel',
  'youtube',
];

// searchURL は、配信サービス service で、作品名 title を探す URL を返す。
export function searchURL(service: StreamingService, title: string): string {
  const q = encodeURIComponent(title);
  switch (service) {
    case 'danime':
      return `https://animestore.docomo.ne.jp/animestore/sch_pc?searchKey=${q}&vodTypeList=svod_tvod`;
    case 'danimeNiconico':
      return `https://ch.nicovideo.jp/search/${q}?channel_id=ch2632720&mode=s&sort=t&order=d&type=video`;
    case 'abema':
      return `https://abema.tv/search?q=${q}`;
    case 'netflix':
      return `https://www.netflix.com/search?q=${q}`;
    case 'primeVideo':
      return `https://www.amazon.co.jp/s?k=${q}&i=instant-video`;
    case 'niconicoChannel':
      return `https://ch.nicovideo.jp/search/${q}?type=channel&mode=s`;
    case 'bandaiChannel':
      return `https://www.b-ch.com/search/text/?search_txt=${q}`;
    case 'youtube':
      return `https://www.youtube.com/results?search_query=${q}`;
  }
}

const storageKey = 'kotatsu.anime.streamingServices';

// loadStreamingServices は、リンクを出す配信サービスを、端末から読む。保存していなければ、すべてを出す。
export function loadStreamingServices(): StreamingService[] {
  return parseStreamingServices(rawSnapshot());
}

// parseStreamingServices は、端末に保存した文字列 raw を、リンクを出す配信サービスにする。読めなければ、すべてを出す。
function parseStreamingServices(raw: string | null): StreamingService[] {
  try {
    if (raw === null) {
      return [...streamingServices];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [...streamingServices];
    }

    // 知らない値 (サービスを減らしたときの古い値) は捨て、並びはいつも同じにする。
    return streamingServices.filter((service) => parsed.includes(service));
  } catch {
    return [...streamingServices];
  }
}

// saveStreamingServices は、リンクを出す配信サービスを、端末に保存する。保存できなくても、今の画面には効かせる。
export function saveStreamingServices(services: readonly StreamingService[]): void {
  try {
    localStorage.setItem(storageKey, JSON.stringify(services));
    preferenceChanged(storageKey);
  } catch {
    // 端末に保存できないだけで、表示は変える。
  }
  window.dispatchEvent(new Event(changeEvent));
}

// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブと、ほかの端末から同期した値 (#1862) にしか届かない。
const changeEvent = 'kotatsu:streaming-services';

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useStreamingServices は、リンクを出す配信サービスと、それを変える関数を返す。ほかのタブや、ほかの端末で変えても追う (#1862)。
// 読んだ文字列を手がかりにするので、値が変わらなければ、描き直さない。
export function useStreamingServices(): [StreamingService[], (next: StreamingService[]) => void] {
  // 読んだ文字列から解く。引数なしで読み直すと、React Compiler が結果をメモ化して使い回すからである。
  return [
    parseStreamingServices(useSyncExternalStore(subscribe, rawSnapshot)),
    saveStreamingServices,
  ];
}

function rawSnapshot(): string | null {
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}
