import {
  currentSeason,
  periodOf,
  periods,
  weekdayOf,
  type AnimeSeason,
  type LibraryBroadcast,
  type LibraryWorkTiming,
  type Period,
} from './library';

// LibraryStatus は、視聴管理の画面 (#1231) で絞り込む作品の状態。既定は「見てる」。
export type LibraryStatus = 'WATCHING' | 'WANNA_WATCH' | 'all';

// libraryStatuses は、絞り込みの並び。
export const libraryStatuses: readonly LibraryStatus[] = ['WATCHING', 'WANNA_WATCH', 'all'];

const seasons: readonly AnimeSeason[] = ['WINTER', 'SPRING', 'SUMMER', 'AUTUMN'];

// LibraryFilters は、視聴管理の画面の絞り込み (#1233)。URL のクエリに持つ (設計書 10.3)。
export type LibraryFilters = {
  readonly status: LibraryStatus;
  // season は、放送の時期。'current' は今期、'2026-AUTUMN' のように年とシーズン。なければ絞らない。
  readonly season: string | undefined;
  // excludeReruns は、再放送を除くかどうか。次の放送と放送の時期を、本放送だけで決める。
  readonly excludeReruns: boolean;
  readonly period: Period | undefined;
  // weekday は、次の放送 (なければ最も新しい放送) の放送日の曜日 (0 が日曜)。
  readonly weekday: number | undefined;
};

// AnimeSearch は、視聴管理の画面の URL のクエリ。既定の値は載せない。
export type AnimeSearch = {
  status?: LibraryStatus;
  season?: string;
  rerun?: 'exclude';
  period?: Period;
  weekday?: number;
};

// parseLibraryStatus は、URL のクエリを、絞り込む状態にする。知らない値は既定 (見てる) にする。
export function parseLibraryStatus(value: unknown): LibraryStatus {
  return libraryStatuses.find((status) => status === value) ?? 'WATCHING';
}

// parseAnimeSearch は、URL のクエリを読む。知らない値は、絞り込まないものとして捨てる。
export function parseAnimeSearch(search: Record<string, unknown>): AnimeSearch {
  const status = parseLibraryStatus(search['status']);
  const season = search['season'];
  const period = periods.find((value) => value === search['period']);
  // 空の値は、Number で 0 (日曜) になるので、数か数字の文字列だけを読む。
  const rawWeekday = search['weekday'];
  const weekday =
    typeof rawWeekday === 'number' || (typeof rawWeekday === 'string' && /^\d$/.test(rawWeekday))
      ? Number(rawWeekday)
      : Number.NaN;

  return {
    ...(status === 'WATCHING' ? {} : { status }),
    ...(typeof season === 'string' && (season === 'current' || parseSeason(season) !== undefined)
      ? { season }
      : {}),
    ...(search['rerun'] === 'exclude' ? { rerun: 'exclude' as const } : {}),
    ...(period === undefined ? {} : { period }),
    ...(Number.isInteger(weekday) && weekday >= 0 && weekday <= 6 ? { weekday } : {}),
  };
}

// filtersOf は、URL のクエリを、絞り込みにする。
export function filtersOf(search: AnimeSearch): LibraryFilters {
  return {
    status: search.status ?? 'WATCHING',
    season: search.season,
    excludeReruns: search.rerun === 'exclude',
    period: search.period,
    weekday: search.weekday,
  };
}

// parseSeason は、'2026-AUTUMN' の形の時期を読む。読めなければ undefined を返す。
export function parseSeason(value: string): { year: number; season: AnimeSeason } | undefined {
  const [year, season] = value.split('-');
  const found = seasons.find((candidate) => candidate === season);
  if (found === undefined || year === undefined || !/^\d{4}$/.test(year)) {
    return undefined;
  }

  return { year: Number(year), season: found };
}

// LibraryFilterWork は、絞り込みに使う作品の項目。
export type LibraryFilterWork<B extends LibraryBroadcast> = LibraryWorkTiming<B> & {
  readonly status: string;
  readonly seasonYear: number | null | undefined;
  readonly season: string | null | undefined;
};

// matchesFilters は、作品が絞り込みに当たるかどうかを返す。
export function matchesFilters<B extends LibraryBroadcast>(
  work: LibraryFilterWork<B>,
  filters: LibraryFilters,
  now: number,
): boolean {
  if (filters.status !== 'all' && work.status !== filters.status) {
    return false;
  }
  if (filters.season !== undefined) {
    const want = filters.season === 'current' ? currentSeason(now) : parseSeason(filters.season);
    if (want === undefined || work.seasonYear !== want.year || work.season !== want.season) {
      return false;
    }
  }
  if (
    filters.period !== undefined &&
    periodOf(work, now, filters.excludeReruns) !== filters.period
  ) {
    return false;
  }
  if (filters.weekday !== undefined && weekdayOf(work, filters.excludeReruns) !== filters.weekday) {
    return false;
  }

  return true;
}
