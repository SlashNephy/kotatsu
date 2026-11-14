// 実況のコメントを、映している場面の時刻に合わせて流す (#1515、設計書 7.4)。
// コメントの書き込まれた時刻と、映しているコマの放送の時刻を比べ、映像がその時刻に届いたときに流す。
// 受け取った時刻で流すと、映像は放送から遅れて映るので、コメントが場面より先に出てしまう。

// lateCommentWindowSeconds は、映している場面の時刻を過ぎて届いたコメントを、まだ流す幅 (設計書 付録 A)。
// 中継が公式のコメントを取り込む分などで遅れて届くので、少しの遅れなら流す。それより古いものは、場面と合わないので流さない。
export const lateCommentWindowSeconds = 5;

// BroadcastNow は、今映しているコマの、放送からの遅れ (秒) と、サーバーの時計での今の時刻 (ミリ秒)。
export type BroadcastNow = { delay: number; serverNow: number };

// syncedCommentWait は、postedAt (Unix ミリ秒) に書き込まれたコメントを、今から何秒後に流すかを返す。
// 映している場面の時刻を lateCommentWindowSeconds より前に過ぎていれば、流さないので null を返す。
export function syncedCommentWait(postedAt: number, broadcast: BroadcastNow): number | null {
  // 映しているコマの放送の時刻は、サーバーの今の時刻から遅れを引いたもの。コメントは、その時刻が postedAt に届いたときに流す。
  const wait = (postedAt + broadcast.delay * 1_000 - broadcast.serverNow) / 1_000;
  if (wait < -lateCommentWindowSeconds) {
    return null;
  }

  return Math.max(wait, 0);
}
