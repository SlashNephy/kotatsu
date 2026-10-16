// 実況のコメントの一覧 (#489、設計書 7.4)。流れていったコメントを読み返し、コピーできるようにする。

import { shouldDraw, type JikkyoComment } from './comments';

// maxListed は、一覧に残すコメントの数の上限。ライブで長く開いていても、メモリと描画を使い切らないようにする。
export const maxListed = 500;

// ListedComment は、一覧の 1 行。at は、コメントの時刻 (Unix ミリ秒)。key は、一覧の中で行を見分ける番号。
export type ListedComment = { key: number; at: number; content: string };

// appendComment は、ライブで届いたコメントを一覧の末尾に足す。流さないコメントは足さない。上限を超えたら、古いものから捨てる。
export function appendComment(
  list: readonly ListedComment[],
  comment: { at: number; content: string; mail: string },
): readonly ListedComment[] {
  if (!shouldDraw({ time: 0, content: comment.content, mail: comment.mail })) {
    return list;
  }
  const key = (list.at(-1)?.key ?? -1) + 1;
  const next = [...list, { key, at: comment.at, content: comment.content }];

  return next.length > maxListed ? next.slice(next.length - maxListed) : next;
}

// commentsUntil は、録画の過去ログ (time の順に並ぶ) のうち、再生位置 position (秒) までに流れたものを、新しいほうから maxListed 件返す。
// 流さないコメントは除く。返す順は、古いものから。
export function commentsUntil(
  comments: readonly JikkyoComment[],
  position: number,
): JikkyoComment[] {
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
  const shown: JikkyoComment[] = [];
  for (let i = lo - 1; i >= 0 && shown.length < maxListed; i--) {
    if (shouldDraw(comments[i]!)) {
      shown.push(comments[i]!);
    }
  }

  return shown.reverse();
}
