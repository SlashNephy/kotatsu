import type { TFunction } from 'i18next';

// FollowingStatus は、フォロー中のユーザーの、作品の状態 (設計書 3.8、#1372)。
export type FollowingStatus =
  | 'WATCHED'
  | 'WATCHING'
  | 'REPEATING'
  | 'ON_HOLD'
  | 'STOP_WATCHING'
  | 'WANNA_WATCH';

// followingStatuses は、状態を並べる順。サーバーが視聴状況を並べる順と同じ。
export const followingStatuses: readonly FollowingStatus[] = [
  'WATCHED',
  'WATCHING',
  'REPEATING',
  'ON_HOLD',
  'STOP_WATCHING',
  'WANNA_WATCH',
];

// followingCounts は、状態ごとの人数を、状態の順に、1 人以上いる状態だけ返す。
export function followingCounts(
  viewings: readonly { readonly status: string }[],
): { status: FollowingStatus; count: number }[] {
  return followingStatuses.flatMap((status) => {
    const count = viewings.filter((viewing) => viewing.status === status).length;

    return count > 0 ? [{ status, count }] : [];
  });
}

// followingDetail は、進捗とスコアを 1 行にする。見終えた作品の進捗は、全話と同じなので出さない。出すものがなければ空にする。
export function followingDetail(
  t: TFunction,
  viewing: {
    readonly status: string;
    readonly progress: number | null | undefined;
    readonly episodesCount: number | null | undefined;
    readonly score: number | null | undefined;
  },
): string {
  const parts: string[] = [];
  if (
    viewing.progress != null &&
    viewing.status !== 'WATCHED' &&
    viewing.progress !== viewing.episodesCount
  ) {
    parts.push(t('anime.following.progress', { count: viewing.progress }));
  }
  if (viewing.score != null) {
    parts.push(t('anime.following.score', { score: viewing.score }));
  }

  return parts.join(' ・ ');
}
