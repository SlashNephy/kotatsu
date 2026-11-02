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

// loadStreamingServices は、リンクを出す配信サービスを、端末から読む。覚えていなければ、すべてを出す。
export function loadStreamingServices(): StreamingService[] {
  try {
    const raw = localStorage.getItem(storageKey);
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

// saveStreamingServices は、リンクを出す配信サービスを、端末に覚える。覚えられなくても、今の画面には効かせる。
export function saveStreamingServices(services: readonly StreamingService[]): void {
  try {
    localStorage.setItem(storageKey, JSON.stringify(services));
  } catch {
    // 端末に覚えられないだけで、表示は変える。
  }
}
