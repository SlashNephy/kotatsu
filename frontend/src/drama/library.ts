import { periodOf, weekdayOf, type LibraryBroadcast, type Period } from '../anime/library';
import { parseAnimeSearch, type LibraryStatus } from '../anime/status';

// ドラマの視聴管理の画面 (#1652、設計書 3.9) の、放送の時期のバッジと絞り込みの決まり。アニメの画面 (#1233) と同じ決まりを使う。

// DramaSearch は、ドラマの視聴管理の画面の URL のクエリ。既定の値 (見ている、絞り込まない) は載せない。
export type DramaSearch = {
  status?: LibraryStatus;
  period?: Period;
  weekday?: number;
};

// DramaFilters は、ドラマの視聴管理の画面の絞り込み。
export type DramaFilters = {
  readonly status: LibraryStatus;
  readonly period: Period | undefined;
  readonly weekday: number | undefined;
};

// parseDramaSearch は、URL のクエリを読む。アニメの画面と同じく、知らない値は、絞り込まないものとして捨てる。
export function parseDramaSearch(search: Record<string, unknown>): DramaSearch {
  const { status, period, weekday } = parseAnimeSearch(search);

  return {
    ...(status === undefined ? {} : { status }),
    ...(period === undefined ? {} : { period }),
    ...(weekday === undefined ? {} : { weekday }),
  };
}

// dramaFiltersOf は、URL のクエリを、絞り込みにする。
export function dramaFiltersOf(search: DramaSearch): DramaFilters {
  return { status: search.status ?? 'WATCHING', period: search.period, weekday: search.weekday };
}

// DramaWorkTiming は、放送の時期を決めるのに使う作品の項目。
export type DramaWorkTiming = {
  readonly status: string;
  readonly nextBroadcast:
    | {
        readonly rerun: boolean;
        readonly last: boolean;
        readonly program: { readonly startAt: string };
      }
    | null
    | undefined;
  readonly recentBroadcast: { readonly startAt: string; readonly last: boolean } | null | undefined;
};

// timingOf は、作品を、アニメの画面の放送の時期の決まりで読める形にする。次の放送はサーバーが決めたものを使う。
function timingOf(work: DramaWorkTiming) {
  const next = work.nextBroadcast;
  const recent = work.recentBroadcast;
  const broadcasts: LibraryBroadcast[] =
    next == null
      ? []
      : [{ countEnd: null, rerun: next.rerun, last: next.last, program: next.program }];

  return {
    progress: null,
    broadcasts,
    recentBroadcasts:
      recent == null ? [] : [{ startAt: recent.startAt, rerun: false, last: recent.last }],
  };
}

// dramaPeriodOf は、作品の放送の時期 (今日、昨日、明日、終了、それより後、未定) を返す。
export function dramaPeriodOf(work: DramaWorkTiming, now: number): Period {
  return periodOf(timingOf(work), now, false);
}

// matchesDramaFilters は、作品が絞り込みに当たるかどうかを返す。
export function matchesDramaFilters(
  work: DramaWorkTiming,
  filters: DramaFilters,
  now: number,
): boolean {
  if (filters.status !== 'all' && work.status !== filters.status) {
    return false;
  }
  if (filters.period !== undefined && dramaPeriodOf(work, now) !== filters.period) {
    return false;
  }
  if (filters.weekday !== undefined && weekdayOf(timingOf(work), false) !== filters.weekday) {
    return false;
  }

  return true;
}
