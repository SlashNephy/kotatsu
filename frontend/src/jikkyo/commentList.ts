// 実況のコメントの一覧 (#489、設計書 7.4)。流れていったコメントを読み返し、コピーできるようにする。

import { shouldDraw, type JikkyoComment } from './comments';

// maxListed は、一覧に残すコメントの数の上限。ライブで長く開いていても、メモリと描画を使い切らないようにする。
export const maxListed = 500;

// ListedComment は、一覧の 1 行。at は、コメントの時刻 (Unix ミリ秒)。key は、一覧の中で行を見分ける番号。
// userId は、投稿したユーザーの匿名の ID。行からミュートするのに使う。分からなければ省く。
export type ListedComment = { key: number; at: number; content: string; userId?: string };

// appendComment は、ライブで届いたコメントを一覧の末尾に足す。流さないコメントは足さない。上限を超えたら、古いものから捨てる。
export function appendComment(
  list: readonly ListedComment[],
  comment: { at: number; content: string; mail: string; userId?: string },
): readonly ListedComment[] {
  if (!shouldDraw({ time: 0, content: comment.content, mail: comment.mail })) {
    return list;
  }
  const key = (list.at(-1)?.key ?? -1) + 1;
  const next = [
    ...list,
    {
      key,
      at: comment.at,
      content: comment.content,
      ...(comment.userId === undefined ? {} : { userId: comment.userId }),
    },
  ];

  return next.length > maxListed ? next.slice(next.length - maxListed) : next;
}

// countUntil は、録画の過去ログ (time の順に並ぶ) のうち、再生位置 position (秒) までに流れたものの数を返す。
export function countUntil(comments: readonly JikkyoComment[], position: number): number {
  // position より後に流れる最初のコメントを、二分探索で探す。
  let lo = 0;
  let hi = comments.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (comments[mid]!.time <= position) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }

  return lo;
}

// listedUntil は、過去ログの先頭から count 件のうち、流すものを新しいほうから maxListed 件、古いものから順に返す。
// index は、過去ログの中の位置。一覧の行を見分けるのに使う。
export function listedUntil(
  comments: readonly JikkyoComment[],
  count: number,
): { index: number; comment: JikkyoComment }[] {
  const shown: { index: number; comment: JikkyoComment }[] = [];
  for (let i = Math.min(count, comments.length) - 1; i >= 0 && shown.length < maxListed; i--) {
    if (shouldDraw(comments[i]!)) {
      shown.push({ index: i, comment: comments[i]! });
    }
  }

  return shown.reverse();
}

// forceWindow は、勢いを数える時間 (ミリ秒)。勢いは、1 分あたりのコメントの数。
const forceWindow = 60_000;
// minForceWindow は、勢いを出すまでに数える最も短い時間 (ミリ秒)。つないだ直後は、少ないコメントから大きく外れた値を出さない。
const minForceWindow = 10_000;

// liveForce は、ライブで届いたコメントの時刻 arrivals (ミリ秒、古い順) から、今 now の勢いを返す (#490)。
// つないでから (since) 1 分たたない間は、それまでに届いた数を 1 分あたりに直す。10 秒たたない間は null を返す。
export function liveForce(arrivals: readonly number[], now: number, since: number): number | null {
  const span = Math.min(now - since, forceWindow);
  if (span < minForceWindow) {
    return null;
  }
  const count = arrivals.filter((at) => at > now - forceWindow && at <= now).length;

  return Math.round((count * forceWindow) / span);
}
