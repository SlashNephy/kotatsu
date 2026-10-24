import { useSyncExternalStore } from 'react';

import { browserStorage } from '../yunomi/volume';

// 番組表の見た目 (密度、通販番組のグレー、ジャンルの色) を、端末ごとに覚えておく (設計書 10.3、#483)。
// 画面の大きさで見やすい密度が変わるので、アカウントには保存しない。

const storageKey = 'kotatsu.guide.appearance';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:guide-appearance';

// densities は、番組表の密度ごとの、チャンネルの幅 (px) と 1 分の高さ (px)。
export const densities = {
  compact: { columnWidth: 136, minuteHeight: 2 },
  normal: { columnWidth: 168, minuteHeight: 3 },
  wide: { columnWidth: 208, minuteHeight: 4 },
} as const;

export type Density = keyof typeof densities;

// genreColorChoices は、ジャンルに選べる色 (Mantine の色の名前)。どれも、淡い版を背景にして、ライトでもダークでも文字が読める。
export const genreColorChoices = [
  'blue',
  'cyan',
  'teal',
  'green',
  'lime',
  'yellow',
  'orange',
  'red',
  'pink',
  'grape',
  'violet',
  'indigo',
  'gray',
] as const;

export type GenreColor = (typeof genreColorChoices)[number];

// colorGenres は、色を選べるジャンル (ARIB STD-B10 の content_nibble の大分類)。拡張 (14) とその他 (15) は灰色のままにする。
export const colorGenres = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11] as const;

// defaultGenreColors は、大分類ごとの既定の色。
export const defaultGenreColors: Readonly<Record<number, GenreColor>> = {
  0: 'blue', // ニュース・報道
  1: 'green', // スポーツ
  2: 'cyan', // 情報・ワイドショー
  3: 'pink', // ドラマ
  4: 'grape', // 音楽
  5: 'yellow', // バラエティ
  6: 'red', // 映画
  7: 'orange', // アニメ・特撮
  8: 'teal', // ドキュメンタリー・教養
  9: 'violet', // 劇場・公演
  10: 'lime', // 趣味・教育
  11: 'indigo', // 福祉
};

export type GuideAppearance = {
  density: Density;
  // grayShopping は、通販番組 (情報・ワイドショーのショッピング・通販) を灰色で出すかどうか。
  grayShopping: boolean;
  // genreColors は、大分類ごとの色。既定から変えたものだけでなく、すべての大分類を持つ。
  genreColors: Readonly<Record<number, GenreColor>>;
};

export const defaultGuideAppearance: GuideAppearance = {
  density: 'normal',
  grayShopping: false,
  genreColors: defaultGenreColors,
};

const isDensity = (value: unknown): value is Density =>
  typeof value === 'string' && Object.hasOwn(densities, value);

const isGenreColor = (value: unknown): value is GenreColor =>
  genreColorChoices.some((color) => color === value);

// parseGuideAppearance は、覚えた値を読む。壊れた項目や、知らない色は、既定に戻す。
export function parseGuideAppearance(raw: string | null): GuideAppearance {
  let value: unknown;
  try {
    value = raw === null ? null : JSON.parse(raw);
  } catch {
    value = null;
  }
  if (typeof value !== 'object' || value === null) {
    return defaultGuideAppearance;
  }
  const record = value as Record<string, unknown>;
  const savedColors =
    typeof record['genreColors'] === 'object' && record['genreColors'] !== null
      ? (record['genreColors'] as Record<string, unknown>)
      : {};
  const genreColors: Record<number, GenreColor> = { ...defaultGenreColors };
  for (const genre of colorGenres) {
    const color = savedColors[String(genre)];
    if (isGenreColor(color)) {
      genreColors[genre] = color;
    }
  }

  return {
    density: isDensity(record['density']) ? record['density'] : defaultGuideAppearance.density,
    grayShopping: record['grayShopping'] === true,
    genreColors,
  };
}

// loadGuideAppearance は、覚えた設定を読む。読めなければ、既定を返す。
export function loadGuideAppearance(storage: Pick<Storage, 'getItem'> | null): GuideAppearance {
  try {
    return parseGuideAppearance(storage?.getItem(storageKey) ?? null);
  } catch {
    return defaultGuideAppearance;
  }
}

// saveGuideAppearance は、設定を覚える。書けなくても、例外は投げない。
export function saveGuideAppearance(
  storage: Pick<Storage, 'setItem'> | null,
  appearance: GuideAppearance,
): void {
  try {
    storage?.setItem(storageKey, JSON.stringify(appearance));
  } catch {
    // 書けなければ、次に開いたときは既定に戻るだけ。
  }
}

// genreBackground は、番組表の枠の背景の色を返す。ジャンルがないか、色を選べないジャンルなら、灰色にする。
// 通販番組を灰色にする設定なら、通販番組も灰色にする。
export function genreBackground(
  appearance: GuideAppearance,
  genre: { lv1: number; lv2: number } | undefined,
): string {
  const shopping = genre?.lv1 === shoppingGenre.lv1 && genre.lv2 === shoppingGenre.lv2;
  const color =
    genre === undefined || (appearance.grayShopping && shopping)
      ? undefined
      : appearance.genreColors[genre.lv1];

  return `var(--mantine-color-${color ?? 'gray'}-light)`;
}

// shoppingGenre は、通販番組のジャンル (情報・ワイドショーのショッピング・通販)。
const shoppingGenre = { lv1: 2, lv2: 4 };

// 読むたびに JSON を解くと、useSyncExternalStore が毎回違う値を受け取って描き直し続けるので、同じ文字列なら前の値を返す。
let cached: { raw: string | null; value: GuideAppearance } | null = null;

function snapshot(): GuideAppearance {
  let raw: string | null = null;
  try {
    raw = browserStorage()?.getItem(storageKey) ?? null;
  } catch {
    raw = null;
  }
  if (cached === null || cached.raw !== raw) {
    cached = { raw, value: parseGuideAppearance(raw) };
  }

  return cached.value;
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useGuideAppearance は、番組表の見た目と、それを変える関数を返す。ほかのタブで変えても追う。
export function useGuideAppearance(): [GuideAppearance, (next: GuideAppearance) => void] {
  const appearance = useSyncExternalStore(subscribe, snapshot);

  return [
    appearance,
    (next) => {
      saveGuideAppearance(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
