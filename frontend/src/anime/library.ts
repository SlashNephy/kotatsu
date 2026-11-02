import { broadcastDay, dateOf } from '../guide/layout';

// 視聴管理の画面 (設計書 3.8、#1233) の、放送の時期のバッジと絞り込みの決まり。画面から切り離して確かめられるようにする。

// AnimeSeason は、作品の放送のシーズン。
export type AnimeSeason = 'WINTER' | 'SPRING' | 'SUMMER' | 'AUTUMN';

// Period は、作品の放送の時期。バッジと絞り込みに使う。
export type Period = 'today' | 'yesterday' | 'tomorrow' | 'ended' | 'later' | 'unknown';

export const periods: readonly Period[] = [
  'ended',
  'yesterday',
  'today',
  'tomorrow',
  'later',
  'unknown',
];

// LibraryBroadcast は、作品のこれからの放送 (放送中を含む)。
export type LibraryBroadcast = {
  readonly countEnd: number | null | undefined;
  readonly rerun: boolean;
  readonly last: boolean;
  readonly program: { readonly startAt: string };
};

// PastBroadcast は、作品の始まった放送。
export type PastBroadcast = {
  readonly startAt: string;
  readonly rerun: boolean;
  readonly last: boolean;
};

// LibraryWorkTiming は、放送の時期を決めるのに使う作品の項目。
export type LibraryWorkTiming<B extends LibraryBroadcast> = {
  readonly progress: number | null | undefined;
  readonly broadcasts: readonly B[];
  readonly recentBroadcasts: readonly PastBroadcast[];
};

const seasonByQuarter: readonly AnimeSeason[] = ['WINTER', 'SPRING', 'SUMMER', 'AUTUMN'];

// currentSeason は、now の放送の時期 (今期) を返す。1〜3 月を冬、4〜6 月を春、7〜9 月を夏、10〜12 月を秋とする (表示のタイムゾーンの日付で)。
export function currentSeason(now: number): { year: number; season: AnimeSeason } {
  const [year, month] = dateOf(now).split('-').map(Number) as [number, number];

  return { year, season: seasonByQuarter[Math.floor((month - 1) / 3)] ?? 'WINTER' };
}

// nextBroadcastOf は、次の放送を返す。見終えた話数より後の話の放送のうち最も早いものを選び、なければ最も早い放送を選ぶ。
// excludeReruns なら、再放送を除いて選ぶ。放送がなければ undefined を返す。
export function nextBroadcastOf<B extends LibraryBroadcast>(
  work: LibraryWorkTiming<B>,
  excludeReruns: boolean,
): B | undefined {
  const broadcasts = excludeReruns ? work.broadcasts.filter((b) => !b.rerun) : work.broadcasts;
  const progress = work.progress;
  if (progress != null) {
    const unwatched = broadcasts.find((b) => b.countEnd != null && b.countEnd > progress);
    if (unwatched !== undefined) {
      return unwatched;
    }
  }

  return broadcasts[0];
}

// recentBroadcastOf は、この 1 週間に始まった最も新しい放送を返す。excludeReruns なら、再放送を除いて選ぶ。
export function recentBroadcastOf(
  work: { readonly recentBroadcasts: readonly PastBroadcast[] },
  excludeReruns: boolean,
): PastBroadcast | undefined {
  return work.recentBroadcasts.find((b) => !excludeReruns || !b.rerun);
}

// periodOf は、作品の放送の時期を返す。日の区切りは、番組表と同じ放送日 (4 時から翌 4 時) にする。
// 今日に始まる (始まった) 放送があれば「今日」、なければ、昨日に始まった放送があれば「昨日」、明日に始まる放送があれば「明日」、
// 最も新しい放送が最終話で、これからの放送がなければ「終了」、これからの放送があれば「それより後」、どれもなければ「未定」にする。
export function periodOf<B extends LibraryBroadcast>(
  work: LibraryWorkTiming<B>,
  now: number,
  excludeReruns: boolean,
): Period {
  const next = nextBroadcastOf(work, excludeReruns);
  const recent = recentBroadcastOf(work, excludeReruns);
  const today = broadcastDay(now);
  const yesterday = broadcastDay(today.from - 1);
  const tomorrow = broadcastDay(today.to);
  const within = (time: number, day: { from: number; to: number }) =>
    time >= day.from && time < day.to;
  const nextStart = next === undefined ? undefined : Date.parse(next.program.startAt);
  const recentStart = recent === undefined ? undefined : Date.parse(recent.startAt);
  if (
    (nextStart !== undefined && nextStart < today.to) ||
    (recentStart !== undefined && within(recentStart, today))
  ) {
    return 'today';
  }
  if (recentStart !== undefined && within(recentStart, yesterday)) {
    return 'yesterday';
  }
  if (nextStart !== undefined && within(nextStart, tomorrow)) {
    return 'tomorrow';
  }
  if (nextStart === undefined && recent?.last === true) {
    return 'ended';
  }

  return nextStart === undefined ? 'unknown' : 'later';
}

// weekdayOf は、次の放送 (なければ最も新しい放送) の、放送日の曜日 (0 が日曜) を返す。放送がなければ undefined を返す。
export function weekdayOf<B extends LibraryBroadcast>(
  work: LibraryWorkTiming<B>,
  excludeReruns: boolean,
): number | undefined {
  const start =
    nextBroadcastOf(work, excludeReruns)?.program.startAt ??
    recentBroadcastOf(work, excludeReruns)?.startAt;
  if (start === undefined) {
    return undefined;
  }

  return new Date(`${dateOf(broadcastDay(Date.parse(start)).from)}T00:00:00Z`).getUTCDay();
}

// sortWorks は、作品を、次の放送の早い順に並べ、放送の予定のない作品を、作品名の順に後ろに回す (サーバーと同じ並び)。
// 再放送を除くと次の放送が変わるので、画面でも並べ直す。
export function sortWorks<
  B extends LibraryBroadcast,
  W extends LibraryWorkTiming<B> & { readonly title: string },
>(works: readonly W[], excludeReruns: boolean): W[] {
  const starts = new Map(
    works.map((work) => [work, nextBroadcastOf(work, excludeReruns)?.program.startAt]),
  );

  return [...works].sort((a, b) => {
    const startA = starts.get(a);
    const startB = starts.get(b);
    if (startA !== undefined && startB !== undefined) {
      return Date.parse(startA) - Date.parse(startB);
    }
    if (startA !== undefined || startB !== undefined) {
      return startA === undefined ? 1 : -1;
    }

    return a.title < b.title ? -1 : a.title > b.title ? 1 : 0;
  });
}
